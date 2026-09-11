package contract

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const TransactionRequestName = "apply-request.json"
const TransactionJournalName = "apply-journal.json"
const InstallJournalName = "install-journal.json"

var transactionTestHook func(string) error

type ApplyRequest struct {
	Schema               int      `json:"schema"`
	Product              string   `json:"product"`
	InstallRoot          string   `json:"install_root"`
	EntryPath            string   `json:"entry_path"`
	StageRecord          string   `json:"stage_record"`
	CurrentPID           int      `json:"current_pid"`
	CurrentExecutable    string   `json:"current_executable"`
	CurrentProcessToken  string   `json:"current_process_token"`
	OwnedChildPID        int      `json:"owned_child_pid,omitempty"`
	OwnedChildExecutable string   `json:"owned_child_executable,omitempty"`
	OwnedChildToken      string   `json:"owned_child_token,omitempty"`
	PriorStateSHA256     string   `json:"prior_state_sha256"`
	RelaunchArguments    []string `json:"relaunch_arguments"`
	AcknowledgementPath  string   `json:"acknowledgement_path"`
	CommitPath           string   `json:"commit_path"`
	ReadyPath            string   `json:"ready_path"`
	Nonce                string   `json:"nonce"`
	WaitTimeoutMS        int      `json:"wait_timeout_ms"`
	CommitTimeoutMS      int      `json:"commit_timeout_ms"`
	ReadyTimeoutMS       int      `json:"ready_timeout_ms"`
}

type ApplyJournal struct {
	Schema         int           `json:"schema"`
	Product        string        `json:"product"`
	Phase          string        `json:"phase"`
	Request        ApplyRequest  `json:"request"`
	Previous       *InstallState `json:"previous,omitempty"`
	Candidate      *InstallState `json:"candidate,omitempty"`
	GenerationDir  string        `json:"generation_dir,omitempty"`
	CandidatePID   int           `json:"candidate_pid,omitempty"`
	CandidateExe   string        `json:"candidate_executable,omitempty"`
	CandidateToken string        `json:"candidate_process_token,omitempty"`
	Error          string        `json:"error,omitempty"`
}

type InstallJournal struct {
	Schema            int          `json:"schema"`
	Product           string       `json:"product"`
	Phase             string       `json:"phase"`
	InstallRoot       string       `json:"install_root"`
	EntryPath         string       `json:"entry_path"`
	GenerationDir     string       `json:"generation_dir,omitempty"`
	Candidate         InstallState `json:"candidate"`
	PriorStateExisted bool         `json:"prior_state_existed"`
	PriorStateSHA256  string       `json:"prior_state_sha256,omitempty"`
	Supersedes        []string     `json:"supersedes,omitempty"`
}

type PreparedApply struct {
	Schema               int    `json:"schema"`
	Product              string `json:"product"`
	TransactionDirectory string `json:"transaction_directory"`
	RequestPath          string `json:"request_path"`
	ManagerPath          string `json:"manager_path"`
	AcknowledgementPath  string `json:"acknowledgement_path"`
	CommitPath           string `json:"commit_path"`
	Nonce                string `json:"nonce"`
}

type ApplyResult struct {
	Schema        int    `json:"schema"`
	Product       string `json:"product"`
	Status        string `json:"status"`
	Version       string `json:"version"`
	VersionPath   string `json:"version_path"`
	RolledBack    bool   `json:"rolled_back"`
	FailureReason string `json:"failure_reason,omitempty"`
}

func PrepareApply(manager string, request ApplyRequest) (PreparedApply, error) {
	root, err := NormalizeInstallRoot(request.InstallRoot)
	if err != nil {
		return PreparedApply{}, err
	}
	request.InstallRoot = root
	request.EntryPath, err = normalizeAbsolutePath(request.EntryPath, "entry path")
	if err != nil {
		return PreparedApply{}, err
	}
	if err = validateInstallTargets(root, request.EntryPath); err != nil {
		return PreparedApply{}, err
	}
	request.CurrentExecutable, err = normalizeAbsolutePath(request.CurrentExecutable, "current executable")
	if err != nil {
		return PreparedApply{}, err
	}
	if request.CurrentPID <= 0 || len(request.RelaunchArguments) > 32 {
		return PreparedApply{}, errors.New("apply process identity or arguments are invalid")
	}
	for _, argument := range request.RelaunchArguments {
		if len(argument) > 4096 || strings.ContainsRune(argument, 0) {
			return PreparedApply{}, errors.New("apply relaunch argument is invalid")
		}
	}
	lock, err := acquireInstallLock(root, 10*time.Second)
	if err != nil {
		return PreparedApply{}, err
	}
	defer lock.Close()
	if err = rejectIncompleteTransactions(root); err != nil {
		return PreparedApply{}, err
	}
	stage, _, err := LoadVerifiedStage(root, request.StageRecord)
	if err != nil {
		return PreparedApply{}, err
	}
	inspection := InspectInstall(root)
	executable, _, activeErr := ActiveExecutable(root)
	if activeErr != nil || !inspection.TrustedIdentity || !samePath(executable, request.CurrentExecutable) {
		return PreparedApply{}, errors.New("running Headroom installation is not a launchable rollback baseline")
	}
	request.CurrentProcessToken, err = captureProcessToken(request.CurrentPID, request.CurrentExecutable)
	if err != nil {
		return PreparedApply{}, err
	}
	if request.OwnedChildPID > 0 {
		request.OwnedChildExecutable, err = normalizeAbsolutePath(request.OwnedChildExecutable, "owned child executable")
		if err != nil {
			return PreparedApply{}, err
		}
		manifestFile, openErr := os.Open(filepath.Join(root, filepath.FromSlash(inspection.VersionPath), PackageManifestName))
		if openErr != nil {
			return PreparedApply{}, openErr
		}
		manifest, decodeErr := DecodePackageManifest(manifestFile)
		manifestFile.Close()
		if decodeErr != nil {
			return PreparedApply{}, decodeErr
		}
		expectedChild := installedComponentPath(root, inspection.VersionPath, manifest.Components.Server.Path)
		if !samePath(request.OwnedChildExecutable, expectedChild) {
			return PreparedApply{}, errors.New("owned child is not the active bundled usage server")
		}
		request.OwnedChildToken, err = captureProcessToken(request.OwnedChildPID, request.OwnedChildExecutable)
		if err != nil {
			return PreparedApply{}, err
		}
	} else if request.OwnedChildExecutable != "" {
		return PreparedApply{}, errors.New("owned child identity is incomplete")
	}
	request.PriorStateSHA256, err = digestFile(filepath.Join(root, StateName))
	if err != nil {
		return PreparedApply{}, err
	}
	if stage.Version == "" {
		return PreparedApply{}, errors.New("verified stage has no version")
	}
	transactions, err := ensureOwnedDirectory(root, "transactions", 0o700)
	if err != nil {
		return PreparedApply{}, err
	}
	token, err := randomHex(16)
	if err != nil {
		return PreparedApply{}, err
	}
	directory := filepath.Join(transactions, "apply-"+token)
	if err = os.Mkdir(directory, 0o700); err != nil {
		return PreparedApply{}, err
	}
	failed := true
	defer func() {
		if failed {
			_ = os.RemoveAll(directory)
		}
	}()
	ext := ""
	if stage.Platform == "windows" {
		ext = ".exe"
	}
	managerCopy := filepath.Join(directory, "headroom-apply"+ext)
	if err = copyFile(manager, managerCopy, 0o700); err != nil {
		return PreparedApply{}, err
	}
	request.Schema, request.Product = SchemaVersion, "Headroom"
	request.StageRecord = filepath.Clean(request.StageRecord)
	request.AcknowledgementPath = filepath.Join(directory, "accepted.json")
	request.CommitPath = filepath.Join(directory, "commit.json")
	request.ReadyPath = filepath.Join(directory, "ready.json")
	request.Nonce, err = randomHex(24)
	if err != nil {
		return PreparedApply{}, err
	}
	if request.WaitTimeoutMS <= 0 || request.WaitTimeoutMS > 120000 {
		request.WaitTimeoutMS = 30000
	}
	if request.CommitTimeoutMS <= 0 || request.CommitTimeoutMS > 30000 {
		request.CommitTimeoutMS = 15000
	}
	if request.ReadyTimeoutMS <= 0 || request.ReadyTimeoutMS > 120000 {
		request.ReadyTimeoutMS = 30000
	}
	requestPath := filepath.Join(directory, TransactionRequestName)
	if err = writeDurableJSON(requestPath, request); err != nil {
		return PreparedApply{}, err
	}
	journal := ApplyJournal{Schema: SchemaVersion, Product: "Headroom", Phase: "prepared", Request: request}
	if err = writeDurableJSON(filepath.Join(directory, TransactionJournalName), journal); err != nil {
		return PreparedApply{}, err
	}
	failed = false
	return PreparedApply{Schema: SchemaVersion, Product: "Headroom", TransactionDirectory: directory,
		RequestPath: requestPath, ManagerPath: managerCopy, AcknowledgementPath: request.AcknowledgementPath,
		CommitPath: request.CommitPath, Nonce: request.Nonce}, nil
}

func rejectIncompleteTransactions(root string) error {
	entries, err := os.ReadDir(filepath.Join(root, "transactions"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "apply-") {
			continue
		}
		journal, readErr := readJournal(filepath.Join(root, "transactions", entry.Name(), TransactionJournalName))
		if readErr == nil && journal.Schema == SchemaVersion && journal.Product == "Headroom" &&
			journal.Phase != "complete" && journal.Phase != "rolled-back" && journal.Phase != "obsolete" {
			return errors.New("another Headroom apply transaction must be recovered before preparing an update")
		}
	}
	return nil
}

func supersedeUnrecoverableTransactions(root string) ([]string, error) {
	current, err := readStateUnchecked(root)
	if err != nil {
		return nil, errors.New("unrecoverable transaction has no valid installed state")
	}
	entries, err := os.ReadDir(filepath.Join(root, "transactions"))
	if err != nil {
		return nil, err
	}
	var superseded []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		directory := filepath.Join(root, "transactions", entry.Name())
		if strings.HasPrefix(entry.Name(), "apply-") {
			journal, readErr := readJournal(filepath.Join(directory, TransactionJournalName))
			if readErr != nil || validateApplyJournal(journal, root, directory) != nil || journal.Phase == "complete" || journal.Phase == "rolled-back" || journal.Phase == "obsolete" {
				continue
			}
			ownsState := journal.Previous != nil && statesEqual(current, *journal.Previous) || journal.Candidate != nil && statesEqual(current, *journal.Candidate)
			if !ownsState {
				continue
			}
			if journal.CandidatePID > 0 {
				if err = stopRecordedProcess(journal.CandidatePID, journal.CandidateExe, journal.CandidateToken, 5*time.Second); err != nil {
					return nil, fmt.Errorf("cannot supersede running update candidate: %w", err)
				}
			}
			superseded = append(superseded, directory)
			continue
		}
		if strings.HasPrefix(entry.Name(), "install-") {
			journal, readErr := readInstallJournal(filepath.Join(directory, InstallJournalName), root, directory)
			if readErr != nil || journal.Phase == "complete" || journal.Phase == "rolled-back" || journal.Phase == "obsolete" {
				continue
			}
			if !statesEqual(current, journal.Candidate) {
				if prior, priorErr := digestFile(filepath.Join(root, StateName)); !journal.PriorStateExisted || priorErr != nil || prior != journal.PriorStateSHA256 {
					continue
				}
			}
			superseded = append(superseded, directory)
		}
	}
	if len(superseded) == 0 {
		return nil, errors.New("recovery failure is not an owned transaction that a verified reinstall can supersede")
	}
	return superseded, nil
}

func WaitForApplyAcknowledgement(prepared PreparedApply, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := readBoundedFile(prepared.AcknowledgementPath, 4096)
		if err == nil {
			var acknowledgement struct {
				Schema   int    `json:"schema"`
				Product  string `json:"product"`
				Accepted bool   `json:"accepted"`
				Nonce    string `json:"nonce"`
			}
			if json.Unmarshal(data, &acknowledgement) == nil && acknowledgement.Schema == SchemaVersion && acknowledgement.Product == "Headroom" && acknowledgement.Accepted && acknowledgement.Nonce == prepared.Nonce {
				return nil
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	return errors.New("transaction manager did not acknowledge the apply request")
}

func waitForApplyCommit(request ApplyRequest, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := readBoundedFile(request.CommitPath, 4096)
		if err == nil {
			var commit struct {
				Schema  int    `json:"schema"`
				Product string `json:"product"`
				Commit  bool   `json:"commit"`
				Nonce   string `json:"nonce"`
			}
			if json.Unmarshal(data, &commit) == nil && commit.Schema == SchemaVersion && commit.Product == "Headroom" && commit.Commit && commit.Nonce == request.Nonce {
				return nil
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	return errors.New("apply authorization was not committed")
}

func LoadVerifiedStage(installRoot, recordPath string) (StageResult, PackageManifest, error) {
	root, err := NormalizeInstallRoot(installRoot)
	if err != nil {
		return StageResult{}, PackageManifest{}, err
	}
	recordPath, err = normalizeAbsolutePath(recordPath, "stage record")
	if err != nil {
		return StageResult{}, PackageManifest{}, err
	}
	staging, err := ensureOwnedDirectory(root, "staging", 0o700)
	if err != nil {
		return StageResult{}, PackageManifest{}, err
	}
	if !isDirectOwnedRecord(staging, recordPath) {
		return StageResult{}, PackageManifest{}, errors.New("verified stage record is outside the owned staging root")
	}
	data, err := readBoundedFile(recordPath, maxManifestBytes)
	if err != nil {
		return StageResult{}, PackageManifest{}, errors.New("verified stage record is unavailable or oversized")
	}
	var stage StageResult
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if decoder.Decode(&stage) != nil || ensureEOF(decoder) != nil || stage.Schema != SchemaVersion || stage.Product != "Headroom" {
		return StageResult{}, PackageManifest{}, errors.New("verified stage record is invalid")
	}
	packageRoot, err := normalizeAbsolutePath(stage.PackageRoot, "staged package root")
	if err != nil || filepath.Dir(filepath.Dir(packageRoot)) != filepath.Dir(recordPath) {
		return StageResult{}, PackageManifest{}, errors.New("staged package root does not match its record")
	}
	if err = validateInstallTargets(packageRoot, ""); err != nil {
		return StageResult{}, PackageManifest{}, err
	}
	manifestFile, err := os.Open(filepath.Join(packageRoot, PackageManifestName))
	if err != nil {
		return StageResult{}, PackageManifest{}, err
	}
	manifest, decodeErr := DecodePackageManifest(manifestFile)
	manifestFile.Close()
	if decodeErr != nil {
		return StageResult{}, PackageManifest{}, decodeErr
	}
	hash, err := digestFile(filepath.Join(packageRoot, PackageManifestName))
	if err != nil || hash != stage.ManifestSHA256 || manifest.Version != stage.Version || manifest.Platform != stage.Platform ||
		manifest.Architecture != stage.Architecture || manifest.AssetName != stage.PackageAsset || manifest.PackageKind != stage.PackageKind {
		return StageResult{}, PackageManifest{}, errors.New("verified stage identity changed")
	}
	if err = VerifyTree(packageRoot, manifest); err != nil {
		return StageResult{}, PackageManifest{}, err
	}
	stage.PackageRoot = packageRoot
	return stage, manifest, nil
}

func ApplyPrepared(requestPath string) (result ApplyResult, returned error) {
	var previous *InstallState
	var journal ApplyJournal
	var childWatch processWatch
	appExited := false
	childExited := false
	complete := false
	recoverFailure := func() {
		if !appExited || complete || returned == nil || previous == nil {
			return
		}
		directory := filepath.Dir(requestPath)
		originalFailure := returned
		originalPhase := journal.Phase
		var cleanupErr error
		journal.Error, journal.Phase = boundedError(originalFailure), "rolling-back"
		if err := writeDurableJSON(filepath.Join(directory, TransactionJournalName), journal); err != nil {
			cleanupErr = fmt.Errorf("record rollback intent: %w", err)
		}
		if journal.CandidatePID > 0 {
			if err := stopRecordedProcess(journal.CandidatePID, journal.CandidateExe, journal.CandidateToken, 5*time.Second); err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("stop candidate: %w", err))
			}
		}
		if childWatch != nil && !childExited {
			if err := childWatch.KillWait(5 * time.Second); err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("stop owned server: %w", err))
			}
		}
		if cleanupErr == nil {
			if journal.Candidate != nil && (originalPhase == "bootstrap-intent" || originalPhase == "bootstrap-ready" || originalPhase == "state-intent" || originalPhase == "activated" || originalPhase == "candidate-launched" || originalPhase == "complete") {
				cleanupErr = rollbackJournal(journal, directory)
			} else if journal.GenerationDir != "" {
				cleanupErr = os.RemoveAll(journal.GenerationDir)
			}
		}
		if cleanupErr == nil {
			current, stateErr := readStateUnchecked(journal.Request.InstallRoot)
			if stateErr != nil || !statesEqual(current, *previous) {
				cleanupErr = errors.New("rollback state could not be verified")
			} else if _, _, activeErr := ActiveExecutable(journal.Request.InstallRoot); activeErr != nil {
				cleanupErr = fmt.Errorf("rollback application could not be verified: %w", activeErr)
			} else if verifyErr := verifyBootstrapRestore(journal.Request.InstallRoot, journal.Request.EntryPath, directory); verifyErr != nil {
				cleanupErr = verifyErr
			}
		}
		if cleanupErr == nil {
			result = ApplyResult{Schema: SchemaVersion, Product: "Headroom", Status: "rolled_back", Version: previous.ActiveVersion,
				VersionPath: previous.VersionPath, RolledBack: true, FailureReason: boundedError(originalFailure)}
			relaunch := journal.Request
			relaunch.ReadyPath = filepath.Join(directory, "rollback-ready.json")
			relaunch.Nonce, _ = randomHex(24)
			if relaunchErr := launchAndAwaitReady(relaunch, *previous, nil); relaunchErr != nil {
				cleanupErr = fmt.Errorf("previous app relaunch failed: %w", relaunchErr)
			}
		}
		if cleanupErr != nil {
			result = ApplyResult{Schema: SchemaVersion, Product: "Headroom", Status: "recovery_required", Version: previous.ActiveVersion,
				VersionPath: previous.VersionPath, FailureReason: boundedError(errors.Join(originalFailure, cleanupErr))}
			if current, stateErr := readStateUnchecked(journal.Request.InstallRoot); stateErr == nil && validVersionPath(current) {
				result.Version, result.VersionPath = current.ActiveVersion, current.VersionPath
			}
			returned = errors.Join(originalFailure, cleanupErr)
		}
		if persistErr := writeDurableJSON(filepath.Join(journal.Request.InstallRoot, "last-apply-result.json"), result); persistErr != nil {
			returned = errors.Join(returned, fmt.Errorf("persist apply outcome: %w", persistErr))
		}
	}
	request, journalPath, err := readApplyRequest(requestPath)
	if err != nil {
		return result, err
	}
	lock, err := acquireInstallLock(request.InstallRoot, 10*time.Second)
	if err != nil {
		return result, err
	}
	defer lock.Close()
	preparedJournal, err := readJournal(journalPath)
	if err != nil || preparedJournal.Phase != "prepared" || !applyRequestsEqual(preparedJournal.Request, request) {
		return result, errors.New("prepared apply journal does not match its request")
	}
	if currentHash, hashErr := digestFile(filepath.Join(request.InstallRoot, StateName)); hashErr != nil || currentHash != request.PriorStateSHA256 {
		return result, errors.New("installed state changed before apply acknowledgement")
	}
	if active, activeInspection, activeErr := ActiveExecutable(request.InstallRoot); activeErr != nil || !activeInspection.TrustedIdentity || !samePath(active, request.CurrentExecutable) {
		return result, errors.New("installed app/runtime changed before apply acknowledgement")
	}
	if _, _, stageErr := LoadVerifiedStage(request.InstallRoot, request.StageRecord); stageErr != nil {
		return result, stageErr
	}
	previous, err = readTrustedState(request.InstallRoot, true)
	if err != nil {
		return result, err
	}
	watch, err := watchProcess(request.CurrentPID, request.CurrentExecutable, request.CurrentProcessToken)
	if err != nil {
		return result, err
	}
	defer watch.Close()
	if request.OwnedChildPID > 0 {
		childWatch, err = watchProcess(request.OwnedChildPID, request.OwnedChildExecutable, request.OwnedChildToken)
		if err != nil {
			return result, err
		}
		defer childWatch.Close()
	}
	defer recoverFailure()
	journal = ApplyJournal{Schema: SchemaVersion, Product: "Headroom", Phase: "waiting-for-exit", Request: request, Previous: previous}
	if err = writeDurableJSON(journalPath, journal); err != nil {
		return result, err
	}
	if err = writeDurableJSON(request.AcknowledgementPath, map[string]any{"schema": SchemaVersion, "product": "Headroom", "accepted": true, "nonce": request.Nonce}); err != nil {
		return result, err
	}
	if err = waitForApplyCommit(request, time.Duration(request.CommitTimeoutMS)*time.Millisecond); err != nil {
		return result, err
	}
	if err = watch.Wait(time.Duration(request.WaitTimeoutMS) * time.Millisecond); err != nil {
		return result, err
	}
	appExited = true
	if childWatch != nil {
		if err = childWatch.Wait(time.Duration(request.WaitTimeoutMS) * time.Millisecond); err != nil {
			return result, err
		}
		childExited = true
	}
	stage, manifest, err := LoadVerifiedStage(request.InstallRoot, request.StageRecord)
	if err != nil {
		return result, err
	}
	journal.Phase = "copying"
	if err = writeDurableJSON(journalPath, journal); err != nil {
		return result, err
	}
	candidate, generationDir, err := createGeneration(request.InstallRoot, stage, manifest)
	if err != nil {
		return result, err
	}
	journal.Candidate, journal.GenerationDir = &candidate, generationDir
	journal.Phase = "generation-ready"
	if err = writeDurableJSON(journalPath, journal); err != nil {
		return result, err
	}
	if err = prepareBootstrapBackup(request.InstallRoot, request.EntryPath, filepath.Dir(journalPath)); err != nil {
		_ = os.RemoveAll(generationDir)
		return result, err
	}
	journal.Phase = "bootstrap-intent"
	if err = writeDurableJSON(journalPath, journal); err != nil {
		_ = os.RemoveAll(generationDir)
		return result, err
	}
	if err = applyBootstrap(stage.PackageRoot, request.InstallRoot, request.EntryPath, manifest); err != nil {
		_ = restoreBootstrap(request.InstallRoot, request.EntryPath, filepath.Dir(journalPath))
		_ = os.RemoveAll(generationDir)
		return result, err
	}
	journal.Phase = "bootstrap-ready"
	if err = writeDurableJSON(journalPath, journal); err != nil {
		_ = restoreBootstrap(request.InstallRoot, request.EntryPath, filepath.Dir(journalPath))
		return result, err
	}
	journal.Phase = "state-intent"
	if err = writeDurableJSON(journalPath, journal); err != nil {
		return result, err
	}
	if err = writeDurableJSON(filepath.Join(request.InstallRoot, StateName), candidate); err != nil {
		_ = restoreBootstrap(request.InstallRoot, request.EntryPath, filepath.Dir(journalPath))
		return result, err
	}
	journal.Phase = "activated"
	if err = writeDurableJSON(journalPath, journal); err != nil {
		return result, err
	}
	if err = launchAndAwaitReady(request, candidate, func(pid int, executable, token string) error {
		journal.CandidatePID, journal.CandidateExe, journal.CandidateToken = pid, executable, token
		journal.Phase = "candidate-launched"
		if err := writeDurableJSON(journalPath, journal); err != nil {
			return err
		}
		if transactionTestHook != nil {
			return transactionTestHook("candidate-launched")
		}
		return nil
	}); err != nil {
		return result, err
	}
	journal.Phase = "complete"
	if err = writeDurableJSON(journalPath, journal); err != nil {
		return result, err
	}
	_ = os.RemoveAll(filepath.Dir(filepath.Dir(stage.PackageRoot)))
	result = ApplyResult{Schema: SchemaVersion, Product: "Headroom", Status: "applied", Version: candidate.ActiveVersion, VersionPath: candidate.VersionPath}
	_ = writeDurableJSON(filepath.Join(request.InstallRoot, "last-apply-result.json"), result)
	complete = true
	return result, nil
}

func verifyBootstrapRestore(root, entry, directory string) error {
	targets := stableBootstrapTargets(root, entry)
	data, err := readBoundedFile(filepath.Join(directory, "bootstrap-backup.json"), maxManifestBytes)
	if err != nil {
		return err
	}
	var records []bootstrapBackupRecord
	if json.Unmarshal(data, &records) != nil || len(records) != len(targets) {
		return errors.New("bootstrap backup record is invalid")
	}
	for index, target := range targets {
		if !records[index].Existed {
			if _, statErr := os.Lstat(target); !errors.Is(statErr, os.ErrNotExist) {
				return errors.New("bootstrap absence was not restored")
			}
			continue
		}
		want, hashErr := digestFile(filepath.Join(directory, fmt.Sprintf("backup-%d", index)))
		got, targetErr := digestFile(target)
		backupInfo, backupErr := os.Lstat(filepath.Join(directory, fmt.Sprintf("backup-%d", index)))
		targetInfo, infoErr := os.Lstat(target)
		if hashErr != nil || targetErr != nil || backupErr != nil || infoErr != nil || want != got || !backupModesEqual(backupInfo.Mode(), targetInfo.Mode()) {
			return errors.New("bootstrap contents were not restored")
		}
	}
	return nil
}

func applyRequestsEqual(left, right ApplyRequest) bool {
	l, _ := json.Marshal(left)
	r, _ := json.Marshal(right)
	return string(l) == string(r)
}

func RecoverInstall(installRoot string) error {
	root, err := ValidateInstallRoot(installRoot)
	if err != nil {
		return err
	}
	lock, err := acquireInstallLock(root, 5*time.Second)
	if err != nil {
		return err
	}
	defer lock.Close()
	transactions := filepath.Join(root, "transactions")
	entries, err := os.ReadDir(transactions)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	deferred := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "install-") {
			continue
		}
		directory := filepath.Join(transactions, entry.Name())
		journal, readErr := readInstallJournal(filepath.Join(directory, InstallJournalName), root, directory)
		if readErr == nil && journal.Phase != "complete" && journal.Phase != "rolled-back" && journal.Phase != "obsolete" {
			for _, name := range journal.Supersedes {
				deferred[name] = true
			}
		}
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if deferred[entry.Name()] {
			continue
		}
		directory := filepath.Join(transactions, entry.Name())
		if strings.HasPrefix(entry.Name(), "install-") {
			journal, readErr := readInstallJournal(filepath.Join(directory, InstallJournalName), root, directory)
			if readErr != nil {
				continue
			}
			if journal.Phase == "complete" || journal.Phase == "rolled-back" {
				_ = os.RemoveAll(directory)
				continue
			}
			if journal.Phase == "obsolete" {
				continue
			}
			if err = recoverInstallJournal(journal, directory); err != nil {
				return err
			}
			continue
		}
		if !strings.HasPrefix(entry.Name(), "apply-") {
			continue
		}
		journal, readErr := readJournal(filepath.Join(directory, TransactionJournalName))
		if readErr != nil {
			continue
		}
		if journal.Phase == "prepared" {
			request, _, requestErr := readApplyRequest(filepath.Join(directory, TransactionRequestName))
			if requestErr == nil && applyRequestsEqual(request, journal.Request) {
				_ = os.RemoveAll(directory)
			}
			continue
		}
		if validateApplyJournal(journal, root, directory) != nil {
			continue
		}
		if journal.Phase == "complete" || journal.Phase == "rolled-back" {
			_ = os.RemoveAll(directory)
			continue
		}
		if journal.CandidatePID > 0 && (journal.Phase == "candidate-launched" || journal.Phase == "activated" || journal.Phase == "rolling-back") {
			if err = stopRecordedProcess(journal.CandidatePID, journal.CandidateExe, journal.CandidateToken, 5*time.Second); err != nil {
				return err
			}
		}
		current, stateErr := readStateUnchecked(root)
		isPrior := stateErr == nil && journal.Previous != nil && statesEqual(current, *journal.Previous)
		isCandidate := stateErr == nil && journal.Candidate != nil && statesEqual(current, *journal.Candidate)
		if journal.Phase == "waiting-for-exit" && isPrior {
			_ = os.RemoveAll(directory)
			continue
		}
		acted := false
		if isCandidate || (isPrior && (journal.Phase == "bootstrap-intent" || journal.Phase == "bootstrap-ready" || journal.Phase == "state-intent")) {
			if err = rollbackJournal(journal, directory); err != nil {
				return err
			}
			acted = true
		} else if isPrior && journal.GenerationDir != "" {
			if err = os.RemoveAll(journal.GenerationDir); err != nil {
				return err
			}
			acted = true
		} else if !isPrior && !isCandidate {
			journal.Phase = "obsolete"
			_ = writeDurableJSON(filepath.Join(directory, TransactionJournalName), journal)
			continue
		}
		journal.Phase = "rolled-back"
		if err = writeDurableJSON(filepath.Join(directory, TransactionJournalName), journal); err != nil {
			return err
		}
		if acted && journal.Previous != nil {
			_ = writeDurableJSON(filepath.Join(root, "last-apply-result.json"), ApplyResult{Schema: SchemaVersion, Product: "Headroom",
				Status: "rolled_back", Version: journal.Previous.ActiveVersion, VersionPath: journal.Previous.VersionPath,
				RolledBack: true, FailureReason: "An interrupted update was rolled back."})
		}
	}
	return nil
}

func readStateUnchecked(root string) (InstallState, error) {
	data, err := readBoundedFile(filepath.Join(root, StateName), maxManifestBytes)
	if err != nil {
		return InstallState{}, err
	}
	var state InstallState
	if json.Unmarshal(data, &state) != nil {
		return InstallState{}, errors.New("installed state is invalid")
	}
	return state, nil
}

func statesEqual(left, right InstallState) bool {
	return left == right
}

func recoverInstallJournal(journal InstallJournal, directory string) error {
	if journal.Phase == "bootstrap-intent" || journal.Phase == "state-intent" {
		statePath := filepath.Join(journal.InstallRoot, StateName)
		currentPrior := false
		if journal.PriorStateExisted {
			digest, err := digestFile(statePath)
			currentPrior = err == nil && digest == journal.PriorStateSHA256
		} else {
			_, err := os.Lstat(statePath)
			currentPrior = errors.Is(err, os.ErrNotExist)
		}
		current, currentErr := readStateUnchecked(journal.InstallRoot)
		currentCandidate := currentErr == nil && statesEqual(current, journal.Candidate)
		if !currentPrior && !currentCandidate {
			journal.Phase = "obsolete"
			return writeDurableJSON(filepath.Join(directory, InstallJournalName), journal)
		}
		if err := restoreBootstrap(journal.InstallRoot, journal.EntryPath, directory); err != nil {
			return err
		}
		if journal.PriorStateExisted {
			if err := replaceFile(filepath.Join(directory, "prior-install-state.json"), statePath, 0o600); err != nil {
				return err
			}
		} else if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if journal.GenerationDir != "" {
		if err := os.RemoveAll(journal.GenerationDir); err != nil {
			return err
		}
	}
	journal.Phase = "rolled-back"
	return writeDurableJSON(filepath.Join(directory, InstallJournalName), journal)
}

func createGeneration(root string, stage StageResult, manifest PackageManifest) (InstallState, string, error) {
	versions, err := ensureOwnedDirectory(root, "versions", 0o755)
	if err != nil {
		return InstallState{}, "", err
	}
	token, err := randomHex(8)
	if err != nil {
		return InstallState{}, "", err
	}
	name := manifest.Version + ".generation-" + stage.ManifestSHA256[:16] + "-" + token
	versionPath := filepath.ToSlash(filepath.Join("versions", name))
	destination := filepath.Join(versions, name)
	if err = copyTree(filepath.Join(stage.PackageRoot, "bundle"), destination); err != nil {
		_ = os.RemoveAll(destination)
		return InstallState{}, "", err
	}
	if err = copyFile(filepath.Join(stage.PackageRoot, PackageManifestName), filepath.Join(destination, PackageManifestName), 0o644); err != nil {
		_ = os.RemoveAll(destination)
		return InstallState{}, "", err
	}
	state := InstallState{PackageKind: manifest.PackageKind, Schema: SchemaVersion, Product: "Headroom", Platform: manifest.Platform, Architecture: manifest.Architecture,
		ActiveVersion: manifest.Version, VersionPath: versionPath, ManifestSHA256: stage.ManifestSHA256, PackageAsset: manifest.AssetName}
	if _, err = inspectState(root, state); err != nil {
		_ = os.RemoveAll(destination)
		return InstallState{}, "", err
	}
	return state, destination, nil
}

func readTrustedState(root string, requireLaunchable bool) (*InstallState, error) {
	data, err := readBoundedFile(filepath.Join(root, StateName), maxManifestBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !requireLaunchable {
			return nil, nil
		}
		return nil, err
	}
	var state InstallState
	if json.Unmarshal(data, &state) != nil {
		return nil, errors.New("installed state is invalid")
	}
	inspection, err := inspectState(root, state)
	if err != nil || !inspection.TrustedIdentity {
		return nil, errors.New("installed state is not trusted")
	}
	if requireLaunchable {
		for _, missing := range inspection.Missing {
			if !isAuxiliaryPath(missing) {
				return nil, errors.New("installed app/runtime is not a rollback baseline")
			}
		}
	}
	return &state, nil
}

func readApplyRequest(path string) (ApplyRequest, string, error) {
	path, err := normalizeAbsolutePath(path, "apply request")
	if err != nil {
		return ApplyRequest{}, "", err
	}
	data, err := readBoundedFile(path, maxManifestBytes)
	if err != nil {
		return ApplyRequest{}, "", errors.New("apply request is unavailable or oversized")
	}
	var request ApplyRequest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if decoder.Decode(&request) != nil || ensureEOF(decoder) != nil || request.Schema != SchemaVersion || request.Product != "Headroom" {
		return ApplyRequest{}, "", errors.New("apply request is invalid")
	}
	root, err := NormalizeInstallRoot(request.InstallRoot)
	if err != nil {
		return ApplyRequest{}, "", err
	}
	transactionRoot := filepath.Join(root, "transactions")
	directoryName := filepath.Base(filepath.Dir(path))
	if filepath.Dir(filepath.Dir(path)) != transactionRoot || !strings.HasPrefix(directoryName, "apply-") || len(directoryName) != 38 || filepath.Base(path) != TransactionRequestName {
		return ApplyRequest{}, "", errors.New("apply request is outside the transaction root")
	}
	request.InstallRoot = root
	directory := filepath.Dir(path)
	if err = validateApplyRequestFields(&request, root, directory); err != nil {
		return ApplyRequest{}, "", err
	}
	return request, filepath.Join(filepath.Dir(path), TransactionJournalName), nil
}

func readJournal(path string) (ApplyJournal, error) {
	data, err := readBoundedFile(path, maxManifestBytes)
	if err != nil {
		return ApplyJournal{}, errors.New("apply journal is unavailable or oversized")
	}
	var journal ApplyJournal
	if json.Unmarshal(data, &journal) != nil || journal.Schema != SchemaVersion || journal.Product != "Headroom" {
		return ApplyJournal{}, errors.New("apply journal is invalid")
	}
	return journal, nil
}

func readInstallJournal(path, root, directory string) (InstallJournal, error) {
	data, err := readBoundedFile(path, maxManifestBytes)
	if err != nil {
		return InstallJournal{}, errors.New("install journal is unavailable or oversized")
	}
	var journal InstallJournal
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if decoder.Decode(&journal) != nil || ensureEOF(decoder) != nil || journal.Schema != SchemaVersion || journal.Product != "Headroom" {
		return InstallJournal{}, errors.New("install journal is invalid")
	}
	if journal.InstallRoot != root || filepath.Dir(directory) != filepath.Join(root, "transactions") ||
		!strings.HasPrefix(filepath.Base(directory), "install-") || len(filepath.Base(directory)) != 24 {
		return InstallJournal{}, errors.New("install journal root is invalid")
	}
	if journal.EntryPath != "" {
		if normalized, pathErr := normalizeAbsolutePath(journal.EntryPath, "entry path"); pathErr != nil || normalized != journal.EntryPath {
			return InstallJournal{}, errors.New("install journal entry is invalid")
		}
	}
	if journal.GenerationDir != filepath.Join(root, filepath.FromSlash(journal.Candidate.VersionPath)) || !validVersionPath(journal.Candidate) {
		return InstallJournal{}, errors.New("install journal generation is invalid")
	}
	if inspection, inspectErr := inspectState(root, journal.Candidate); inspectErr != nil || !inspection.TrustedIdentity {
		return InstallJournal{}, errors.New("install journal candidate is invalid")
	}
	validPhases := map[string]bool{"generation-ready": true, "bootstrap-intent": true, "state-intent": true, "complete": true, "rolled-back": true, "obsolete": true}
	if !validPhases[journal.Phase] {
		return InstallJournal{}, errors.New("install journal phase is invalid")
	}
	if journal.PriorStateExisted {
		if !hashPattern.MatchString(journal.PriorStateSHA256) {
			return InstallJournal{}, errors.New("install journal prior state hash is invalid")
		}
		if info, stateErr := os.Lstat(filepath.Join(directory, "prior-install-state.json")); stateErr != nil || !info.Mode().IsRegular() {
			return InstallJournal{}, errors.New("install journal prior state is invalid")
		}
	} else if journal.PriorStateSHA256 != "" {
		return InstallJournal{}, errors.New("install journal prior state identity is invalid")
	}
	for _, name := range journal.Supersedes {
		if !(strings.HasPrefix(name, "apply-") && len(name) == 38 || strings.HasPrefix(name, "install-") && len(name) == 24) || filepath.Base(name) != name {
			return InstallJournal{}, errors.New("install journal superseded transaction is invalid")
		}
	}
	return journal, nil
}

func validateApplyJournal(journal ApplyJournal, root, directory string) error {
	if journal.Request.InstallRoot != root || filepath.Dir(directory) != filepath.Join(root, "transactions") || journal.Previous == nil {
		return errors.New("apply journal root is invalid")
	}
	if !validVersionPath(*journal.Previous) {
		return errors.New("apply rollback state is invalid")
	}
	validPhases := map[string]bool{"waiting-for-exit": true, "copying": true, "generation-ready": true, "bootstrap-intent": true,
		"bootstrap-ready": true, "state-intent": true, "activated": true, "candidate-launched": true, "rolling-back": true,
		"complete": true, "rolled-back": true, "obsolete": true}
	if !validPhases[journal.Phase] {
		return errors.New("apply journal phase is invalid")
	}
	if err := validateApplyRequestFields(&journal.Request, root, directory); err != nil {
		return err
	}
	previousInspection, previousErr := inspectState(root, *journal.Previous)
	if previousErr != nil || !previousInspection.TrustedIdentity {
		return errors.New("apply rollback identity is invalid")
	}
	previousManifest, manifestErr := installedManifest(root, *journal.Previous)
	if manifestErr != nil {
		return errors.New("apply prior manifest is invalid")
	}
	if !samePath(journal.Request.CurrentExecutable, installedComponentPath(root, journal.Previous.VersionPath, previousManifest.Components.Application.Path)) {
		return errors.New("apply prior executable is invalid")
	}
	if journal.Request.OwnedChildPID > 0 {
		if !samePath(journal.Request.OwnedChildExecutable, installedComponentPath(root, journal.Previous.VersionPath, previousManifest.Components.Server.Path)) {
			return errors.New("apply owned child executable is invalid")
		}
	}
	if journal.Candidate != nil {
		expected := filepath.Join(root, filepath.FromSlash(journal.Candidate.VersionPath))
		if !validVersionPath(*journal.Candidate) || journal.GenerationDir != expected {
			return errors.New("apply candidate path is invalid")
		}
		if candidateInspection, candidateErr := inspectState(root, *journal.Candidate); candidateErr != nil || !candidateInspection.TrustedIdentity {
			return errors.New("apply candidate identity is invalid")
		}
	}
	if journal.CandidatePID > 0 && (journal.CandidateExe == "" || journal.CandidateToken == "") {
		return errors.New("apply candidate process identity is invalid")
	}
	if journal.CandidatePID > 0 && journal.Candidate != nil {
		candidateManifest, candidateErr := installedManifest(root, *journal.Candidate)
		if candidateErr != nil || !samePath(journal.CandidateExe, installedComponentPath(root, journal.Candidate.VersionPath, candidateManifest.Components.Application.Path)) {
			return errors.New("apply candidate executable is invalid")
		}
	}
	return nil
}

func validateApplyRequestFields(request *ApplyRequest, root, directory string) error {
	var err error
	request.EntryPath, err = normalizeAbsolutePath(request.EntryPath, "entry path")
	if err != nil {
		return err
	}
	if err = validateInstallTargets(root, request.EntryPath); err != nil {
		return err
	}
	if err = validateInstallTargets(directory, ""); err != nil {
		return err
	}
	request.CurrentExecutable, err = normalizeAbsolutePath(request.CurrentExecutable, "current executable")
	if err != nil || request.CurrentPID <= 0 || request.CurrentProcessToken == "" || len(request.RelaunchArguments) > 32 {
		return errors.New("apply process identity is invalid")
	}
	if request.OwnedChildPID > 0 {
		request.OwnedChildExecutable, err = normalizeAbsolutePath(request.OwnedChildExecutable, "owned child executable")
		if err != nil || request.OwnedChildToken == "" {
			return errors.New("owned child identity is invalid")
		}
	} else if request.OwnedChildExecutable != "" || request.OwnedChildToken != "" {
		return errors.New("owned child identity is incomplete")
	}
	for _, argument := range request.RelaunchArguments {
		if len(argument) > 4096 || strings.ContainsRune(argument, 0) {
			return errors.New("apply relaunch argument is invalid")
		}
	}
	if request.InstallRoot != root || request.WaitTimeoutMS <= 0 || request.WaitTimeoutMS > 120000 || request.CommitTimeoutMS <= 0 || request.CommitTimeoutMS > 30000 || request.ReadyTimeoutMS <= 0 || request.ReadyTimeoutMS > 120000 ||
		len(request.Nonce) != 48 || !isLowerHex(request.Nonce) || !hashPattern.MatchString(request.PriorStateSHA256) ||
		!samePath(request.AcknowledgementPath, filepath.Join(directory, "accepted.json")) || !samePath(request.CommitPath, filepath.Join(directory, "commit.json")) ||
		!samePath(request.ReadyPath, filepath.Join(directory, "ready.json")) {
		return errors.New("apply request bounds or private paths are invalid")
	}
	return nil
}

func isLowerHex(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}

func randomHex(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func boundedError(err error) string {
	value := err.Error()
	if len(value) > 512 {
		return value[:512]
	}
	return value
}

func samePath(left, right string) bool {
	if runtimeWindows() {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func isDirectOwnedRecord(staging, record string) bool {
	directory := filepath.Dir(record)
	return filepath.Dir(directory) == staging && strings.HasPrefix(filepath.Base(directory), "package-") && filepath.Base(record) == "verified-stage.json"
}

func ensureOwnedDirectory(root, child string, mode os.FileMode) (string, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	if err := validateInstallTargets(root, ""); err != nil {
		return "", err
	}
	path := filepath.Join(root, child)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err = os.Mkdir(path, mode); err != nil {
			return "", err
		}
		return path, nil
	}
	if err != nil || !info.IsDir() || pathIsLinkOrReparse(path, info) {
		return "", fmt.Errorf("owned install directory is unsafe: %s", path)
	}
	return path, nil
}

type bootstrapBackupRecord struct {
	Existed bool   `json:"existed"`
	Mode    uint32 `json:"mode"`
}

func stableBootstrapTargets(root, entry string) []string {
	ext := ""
	launcherName := "headroom-launcher"
	if runtimeWindows() {
		ext = ".exe"
		launcherName = "headroom"
	}
	launcher := filepath.Join(root, launcherName+ext)
	values := []string{launcher, filepath.Join(root, "headroom-package"+ext), launcher + ".root"}
	if entry != "" && !samePath(entry, launcher) {
		values = append(values, entry, entry+".root")
	}
	return values
}

func prepareBootstrapBackup(root, entry, backupDir string) error {
	targets := stableBootstrapTargets(root, entry)
	records := make([]bootstrapBackupRecord, len(targets))
	for index, target := range targets {
		info, err := os.Lstat(target)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("stable entry is unsafe: %s", target)
		}
		records[index] = bootstrapBackupRecord{Existed: true, Mode: uint32(info.Mode().Perm())}
		if err = copyFile(target, filepath.Join(backupDir, fmt.Sprintf("backup-%d", index)), info.Mode().Perm()); err != nil {
			return err
		}
	}
	if err := writeDurableJSON(filepath.Join(backupDir, "bootstrap-backup.json"), records); err != nil {
		return err
	}
	return nil
}

func applyBootstrap(packageRoot, root, entry string, manifest PackageManifest) error {
	targets := stableBootstrapTargets(root, entry)
	ext := ""
	launcherName := "headroom"
	if runtimeWindows() {
		ext = ".exe"
	}
	launcherSource := filepath.Join(packageRoot, "bootstrap", launcherName+ext)
	managerSource := filepath.Join(packageRoot, "bootstrap", "headroom-package"+ext)
	sources := []string{launcherSource, managerSource, "association"}
	if len(targets) > 3 {
		sources = append(sources, launcherSource, "association")
	}
	for index, target := range targets {
		if sources[index] == "association" {
			if err := replaceBytes([]byte(root+"\n"), target, 0o600); err != nil {
				return err
			}
		} else if err := replaceFile(sources[index], target, 0o755); err != nil {
			return err
		}
	}
	return verifyInstalledBootstrap(root, entry, manifest)
}

func installBootstrap(packageRoot, root, entry, backupDir string) error {
	if err := prepareBootstrapBackup(root, entry, backupDir); err != nil {
		return err
	}
	manifestFile, err := os.Open(filepath.Join(packageRoot, PackageManifestName))
	if err != nil {
		return err
	}
	manifest, err := DecodePackageManifest(manifestFile)
	manifestFile.Close()
	if err != nil {
		return err
	}
	return applyBootstrap(packageRoot, root, entry, manifest)
}

func verifyInstalledBootstrap(root, entry string, manifest PackageManifest) error {
	ext := ""
	if manifest.Platform == "windows" {
		ext = ".exe"
	}
	find := func(path string) (File, bool) {
		for _, record := range manifest.Files {
			if record.Path == path {
				return record, true
			}
		}
		return File{}, false
	}
	launcherRecord, launcherOK := find("bootstrap/headroom" + ext)
	managerRecord, managerOK := find("bootstrap/headroom-package" + ext)
	if !launcherOK || !managerOK {
		return errors.New("verified package bootstrap inventory is incomplete")
	}
	launcher := filepath.Join(root, "headroom-launcher")
	if manifest.Platform == "windows" {
		launcher = filepath.Join(root, "headroom.exe")
	}
	checks := []struct {
		path   string
		record File
	}{{launcher, launcherRecord}, {filepath.Join(root, "headroom-package"+ext), managerRecord}}
	if entry != "" && !samePath(entry, launcher) {
		checks = append(checks, struct {
			path   string
			record File
		}{entry, launcherRecord})
	}
	for _, check := range checks {
		info, err := os.Lstat(check.path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != check.record.Size {
			return fmt.Errorf("installed bootstrap is incomplete: %s", check.path)
		}
		if manifest.Platform != "windows" && fmt.Sprintf("%04o", info.Mode().Perm()) != check.record.Mode {
			return fmt.Errorf("installed bootstrap mode mismatch: %s", check.path)
		}
		digest, err := digestFile(check.path)
		if err != nil || digest != check.record.SHA256 {
			return fmt.Errorf("installed bootstrap hash mismatch: %s", check.path)
		}
	}
	associations := []string{launcher + ".root"}
	if entry != "" && !samePath(entry, launcher) {
		associations = append(associations, entry+".root")
	}
	for _, association := range associations {
		data, err := readBoundedFile(association, 4096)
		if err != nil || string(data) != root+"\n" {
			return fmt.Errorf("installed bootstrap association is invalid: %s", association)
		}
	}
	return nil
}

func restoreBootstrap(root, entry, backupDir string) error {
	targets := stableBootstrapTargets(root, entry)
	data, err := readBoundedFile(filepath.Join(backupDir, "bootstrap-backup.json"), maxManifestBytes)
	if err != nil {
		return err
	}
	var records []bootstrapBackupRecord
	if json.Unmarshal(data, &records) != nil || len(records) != len(targets) {
		return errors.New("bootstrap backup record is invalid")
	}
	for index, target := range targets {
		if records[index].Existed {
			mode := os.FileMode(records[index].Mode)
			if !validBackupMode(mode) {
				return errors.New("bootstrap backup mode is invalid")
			}
			if err = replaceFile(filepath.Join(backupDir, fmt.Sprintf("backup-%d", index)), target, mode); err != nil {
				return err
			}
		} else if err = os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func rollbackJournal(journal ApplyJournal, directory string) error {
	if journal.Previous == nil || journal.Candidate == nil {
		return errors.New("apply journal has no rollback state")
	}
	if err := restoreBootstrap(journal.Request.InstallRoot, journal.Request.EntryPath, directory); err != nil {
		return err
	}
	if err := writeDurableJSON(filepath.Join(journal.Request.InstallRoot, StateName), journal.Previous); err != nil {
		return err
	}
	if journal.GenerationDir != "" {
		expected := filepath.Join(journal.Request.InstallRoot, filepath.FromSlash(journal.Candidate.VersionPath))
		if !samePath(journal.GenerationDir, expected) {
			return errors.New("journal generation path is invalid")
		}
		if transactionTestHook != nil {
			if err := transactionTestHook("rollback-remove-generation"); err != nil {
				return err
			}
		}
		if err := os.RemoveAll(expected); err != nil {
			return err
		}
	}
	journal.Phase = "rolled-back"
	return writeDurableJSON(filepath.Join(directory, TransactionJournalName), journal)
}

func launchAndAwaitReady(request ApplyRequest, state InstallState, onLaunch func(int, string, string) error) error {
	inspection, err := inspectState(request.InstallRoot, state)
	if err != nil {
		return err
	}
	manifest, err := installedManifest(request.InstallRoot, state)
	if err != nil {
		return err
	}
	executable := installedComponentPath(request.InstallRoot, state.VersionPath, manifest.Components.Application.Path)
	for _, missing := range inspection.Missing {
		if !isAuxiliaryPath(missing) {
			return errors.New("target app/runtime is incomplete")
		}
	}
	_ = os.Remove(request.ReadyPath)
	environment := authoritativeApplyEnvironment(os.Environ(), map[string]string{
		"HEADROOM_INSTALL_ROOT": request.InstallRoot, "HEADROOM_LAUNCHER_PATH": request.EntryPath,
		"HEADROOM_PACKAGE_VERSION": state.ActiveVersion, "HEADROOM_READY_NONCE": request.Nonce,
	})
	arguments := append([]string{executable}, request.RelaunchArguments...)
	arguments = append(arguments, "--headroom-update-restart", "--headroom-ready-file", request.ReadyPath)
	process, err := os.StartProcess(executable, arguments, &os.ProcAttr{Env: environment, Files: []*os.File{nil, nil, nil}})
	if err != nil {
		return err
	}
	token, tokenErr := captureProcessToken(process.Pid, executable)
	if tokenErr != nil {
		_ = process.Kill()
		_, _ = process.Wait()
		return tokenErr
	}
	done := make(chan error, 1)
	go func() { _, waitErr := process.Wait(); done <- waitErr }()
	if onLaunch != nil {
		if err = onLaunch(process.Pid, executable, token); err != nil {
			_ = process.Kill()
			<-done
			return err
		}
	}
	deadline := time.Now().Add(time.Duration(request.ReadyTimeoutMS) * time.Millisecond)
	for time.Now().Before(deadline) {
		select {
		case waitErr := <-done:
			return fmt.Errorf("replacement app exited before readiness: %v", waitErr)
		default:
		}
		data, readErr := readBoundedFile(request.ReadyPath, 4096)
		if readErr == nil {
			var ready struct {
				Nonce      string `json:"nonce"`
				PID        int    `json:"pid"`
				Version    string `json:"version"`
				Executable string `json:"executable"`
			}
			if json.Unmarshal(data, &ready) == nil && ready.Nonce == request.Nonce && ready.PID == process.Pid && ready.Version == state.ActiveVersion && samePath(ready.Executable, executable) {
				select {
				case waitErr := <-done:
					return fmt.Errorf("replacement app exited during readiness: %v", waitErr)
				case <-time.After(200 * time.Millisecond):
					return nil
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	_ = process.Kill()
	<-done
	return errors.New("replacement app readiness timed out")
}

func authoritativeApplyEnvironment(base []string, values map[string]string) []string {
	managed := map[string]bool{"HEADROOM_INSTALL_ROOT": true, "HEADROOM_LAUNCHER_PATH": true, "HEADROOM_PACKAGE_VERSION": true, "HEADROOM_READY_NONCE": true}
	result := make([]string, 0, len(base)+len(values))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		upper := strings.ToUpper(key)
		if !managed[upper] && upper != "HEADROOM_APPLY_READY" {
			result = append(result, item)
		}
	}
	for _, key := range []string{"HEADROOM_INSTALL_ROOT", "HEADROOM_LAUNCHER_PATH", "HEADROOM_PACKAGE_VERSION", "HEADROOM_READY_NONCE"} {
		result = append(result, key+"="+values[key])
	}
	return result
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > limit {
		return nil, errors.New("file is unavailable or oversized")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("file is unavailable or oversized")
	}
	return data, nil
}

func writeDurableJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".headroom-json-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	if err = file.Chmod(0o600); err != nil {
		file.Close()
		_ = os.Remove(temporary)
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err = replaceAtomic(temporary, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
