package contract

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

const maxPackageFileBytes = int64(512 << 20)
const maxPackageBytes = int64(2 << 30)
const maxPackageEntries = 20000

type archiveEntry struct {
	name      string
	size      int64
	mode      os.FileMode
	directory bool
	link      string
}

func InspectArchive(filename string) (PackageManifest, string, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return PackageManifest{}, "", err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > MaxArchiveBytes {
		return PackageManifest{}, "", fmt.Errorf("package archive size is outside the supported range")
	}
	var manifest PackageManifest
	var root string
	if strings.HasSuffix(filename, ".zip") {
		manifest, root, err = inspectZip(filename)
	} else if strings.HasSuffix(filename, ".tar.gz") {
		manifest, root, err = inspectTar(filename)
	} else {
		return PackageManifest{}, "", fmt.Errorf("unsupported package archive")
	}
	if err == nil && filepath.Base(filename) != manifest.AssetName {
		return manifest, "", fmt.Errorf("archive filename %q does not match manifest asset %q", filepath.Base(filename), manifest.AssetName)
	}
	return manifest, root, err
}

func inspectZip(filename string) (PackageManifest, string, error) {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return PackageManifest{}, "", err
	}
	defer reader.Close()
	entries := make([]archiveEntry, 0, len(reader.File))
	roots := map[string]bool{}
	seen := map[string]bool{}
	var manifest PackageManifest
	found := false
	if len(reader.File) > maxPackageEntries {
		return PackageManifest{}, "", fmt.Errorf("archive has too many entries")
	}
	var expanded uint64
	for _, file := range reader.File {
		name := strings.TrimSuffix(file.Name, "/")
		if !validArchiveName(name) || seen[name] {
			return manifest, "", fmt.Errorf("unsafe or duplicate archive entry %q", file.Name)
		}
		seen[name] = true
		roots[strings.SplitN(name, "/", 2)[0]] = true
		if file.Mode()&os.ModeSymlink != 0 || file.Mode()&os.ModeType != 0 && !file.FileInfo().IsDir() {
			return manifest, "", fmt.Errorf("archive links and special entries are forbidden: %s", name)
		}
		if file.UncompressedSize64 > uint64(maxPackageFileBytes) || expanded > uint64(maxPackageBytes)-file.UncompressedSize64 {
			return manifest, "", fmt.Errorf("archive expanded size limit exceeded at %s", name)
		}
		expanded += file.UncompressedSize64
		entries = append(entries, archiveEntry{name: name, size: int64(file.UncompressedSize64), mode: file.Mode(), directory: file.FileInfo().IsDir()})
		if strings.HasSuffix(name, "/"+PackageManifestName) {
			if found {
				return manifest, "", fmt.Errorf("multiple package manifests")
			}
			r, err := file.Open()
			if err != nil {
				return manifest, "", err
			}
			manifest, err = DecodePackageManifest(r)
			r.Close()
			if err != nil {
				return manifest, "", err
			}
			found = true
		}
	}
	return finishInspection(manifest, found, roots, entries)
}

func inspectTar(filename string) (PackageManifest, string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return PackageManifest{}, "", err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return PackageManifest{}, "", err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	roots := map[string]bool{}
	seen := map[string]bool{}
	entries := []archiveEntry{}
	var manifest PackageManifest
	found := false
	count := 0
	total := int64(0)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return manifest, "", err
		}
		count++
		if count > maxPackageEntries {
			return manifest, "", fmt.Errorf("archive has too many entries")
		}
		name := strings.TrimSuffix(header.Name, "/")
		if !validArchiveName(name) || seen[name] {
			return manifest, "", fmt.Errorf("unsafe or duplicate archive entry %q", header.Name)
		}
		seen[name] = true
		roots[strings.SplitN(name, "/", 2)[0]] = true
		directory := header.Typeflag == tar.TypeDir
		link := ""
		if header.Typeflag == tar.TypeSymlink {
			link = header.Linkname
		}
		if !directory && link == "" && header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return manifest, "", fmt.Errorf("archive links and special entries are forbidden: %s", name)
		}
		if header.Size < 0 || header.Size > maxPackageFileBytes || total > maxPackageBytes-header.Size {
			return manifest, "", fmt.Errorf("archive entry has invalid size: %s", name)
		}
		total += header.Size
		entries = append(entries, archiveEntry{name: name, size: header.Size, mode: os.FileMode(header.Mode), directory: directory, link: link})
		if strings.HasSuffix(name, "/"+PackageManifestName) {
			if found {
				return manifest, "", fmt.Errorf("multiple package manifests")
			}
			manifest, err = DecodePackageManifest(reader)
			if err != nil {
				return manifest, "", err
			}
			found = true
		}
	}
	return finishInspection(manifest, found, roots, entries)
}

func finishInspection(manifest PackageManifest, found bool, roots map[string]bool, entries []archiveEntry) (PackageManifest, string, error) {
	if !found || len(roots) != 1 {
		return manifest, "", fmt.Errorf("archive must contain exactly one package root and manifest")
	}
	var root string
	for value := range roots {
		root = value
	}
	expected, _ := ArchiveRootForKind(manifest.PackageKind, manifest.Version, manifest.Platform, manifest.Architecture)
	if root != expected {
		return manifest, "", fmt.Errorf("archive root %q does not match %q", root, expected)
	}
	records := map[string]File{}
	for _, f := range manifest.Files {
		records[f.Path] = f
	}
	folded := map[string]bool{}
	for _, entry := range entries {
		fold := strings.ToLower(entry.name)
		if folded[fold] {
			return manifest, "", fmt.Errorf("case-colliding archive entry %s", entry.name)
		}
		folded[fold] = true
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		rel := strings.TrimPrefix(entry.name, root+"/")
		if rel == entry.name || rel == "" || entry.directory {
			continue
		}
		if rel == PackageManifestName {
			continue
		}
		record, ok := records[rel]
		if !ok {
			if entry.link != "" {
				return manifest, "", fmt.Errorf("archive links are forbidden outside declared macOS frameworks: %s", rel)
			}
			return manifest, "", fmt.Errorf("unlisted archive file %s", rel)
		}
		if entry.link != "" {
			if manifest.Platform != "macos" || record.LinkTarget != entry.link {
				return manifest, "", fmt.Errorf("unlisted archive link %s", rel)
			}
		} else if record.LinkTarget != "" {
			return manifest, "", fmt.Errorf("framework link stored as regular file: %s", rel)
		} else if entry.size != record.Size {
			return manifest, "", fmt.Errorf("size mismatch for %s", rel)
		}
		seen[rel] = true
	}
	for _, entry := range entries {
		if !entry.directory && entry.link == "" {
			rel := strings.TrimPrefix(entry.name, root+"/")
			if record, ok := records[rel]; ok && manifest.Platform != "windows" && record.Mode != fmt.Sprintf("%04o", entry.mode.Perm()) {
				return manifest, "", fmt.Errorf("mode mismatch for %s", rel)
			}
		}
	}
	for rel := range records {
		if !seen[rel] {
			return manifest, "", fmt.Errorf("manifest file missing from archive: %s", rel)
		}
	}
	return manifest, root, nil
}

func validArchiveName(name string) bool {
	return canonicalRelative(name) && !strings.Contains(name, "//")
}

func ExtractAndVerify(filename, destination string, expected Expectations) (PackageManifest, string, error) {
	manifest, root, err := InspectArchive(filename)
	if err != nil {
		return manifest, "", err
	}
	if err = CheckExpectations(manifest, expected); err != nil {
		return manifest, "", err
	}
	if !filepath.IsAbs(destination) {
		return manifest, "", fmt.Errorf("extraction destination must be absolute")
	}
	if _, statErr := os.Lstat(destination); !os.IsNotExist(statErr) {
		return manifest, "", fmt.Errorf("extraction destination must not exist")
	}
	if err = os.Mkdir(destination, 0o700); err != nil {
		return manifest, "", err
	}
	if strings.HasSuffix(filename, ".zip") {
		err = extractZip(filename, destination)
	} else {
		err = extractTar(filename, destination, manifest, root)
	}
	if err != nil {
		return manifest, "", err
	}
	packageRoot := filepath.Join(destination, filepath.FromSlash(root))
	extractedFile, openErr := os.Open(filepath.Join(packageRoot, PackageManifestName))
	if openErr != nil {
		return manifest, "", openErr
	}
	extractedManifest, decodeErr := DecodePackageManifest(extractedFile)
	extractedFile.Close()
	if decodeErr != nil || !reflect.DeepEqual(manifest, extractedManifest) {
		return manifest, "", fmt.Errorf("extracted package manifest does not match inspected manifest")
	}
	if err = VerifyTree(packageRoot, manifest); err != nil {
		return manifest, "", err
	}
	return manifest, packageRoot, nil
}

// VerifyArchive performs the same content, mode, and executable-architecture
// checks as staging, but removes its private extraction immediately.
func VerifyArchive(filename string, expected Expectations) (PackageManifest, string, error) {
	parent, err := os.MkdirTemp("", ".headroom-verify-")
	if err != nil {
		return PackageManifest{}, "", err
	}
	defer os.RemoveAll(parent)
	manifest, root, err := ExtractAndVerify(filename, filepath.Join(parent, "contents"), expected)
	if err != nil {
		return manifest, "", err
	}
	return manifest, filepath.Base(root), nil
}

func extractZip(filename, destination string) error {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return err
	}
	defer reader.Close()
	if len(reader.File) > maxPackageEntries {
		return fmt.Errorf("archive has too many entries")
	}
	var total uint64
	seen := map[string]bool{}
	for _, file := range reader.File {
		name := strings.TrimSuffix(file.Name, "/")
		fold := strings.ToLower(name)
		if !validArchiveName(name) || seen[fold] || file.Mode()&os.ModeSymlink != 0 || file.Mode()&os.ModeType != 0 && !file.FileInfo().IsDir() {
			return fmt.Errorf("unsafe archive entry %q", file.Name)
		}
		seen[fold] = true
		if file.UncompressedSize64 > uint64(maxPackageFileBytes) || total > uint64(maxPackageBytes)-file.UncompressedSize64 {
			return fmt.Errorf("archive expanded size limit exceeded")
		}
		total += file.UncompressedSize64
		target := filepath.Join(destination, filepath.FromSlash(name))
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		mode := file.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		err = copyExclusive(target, src, mode)
		src.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTar(filename, destination string, manifest PackageManifest, archiveRoot string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	count := 0
	total := int64(0)
	seen := map[string]bool{}
	type pendingLink struct{ name, target string }
	var links []pendingLink
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		count++
		name := strings.TrimSuffix(header.Name, "/")
		fold := strings.ToLower(name)
		directory := header.Typeflag == tar.TypeDir
		link := header.Typeflag == tar.TypeSymlink
		if count > maxPackageEntries || !validArchiveName(name) || seen[fold] || !directory && !link && header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA || header.Size < 0 || header.Size > maxPackageFileBytes || total > maxPackageBytes-header.Size {
			return fmt.Errorf("unsafe archive entry %q", header.Name)
		}
		seen[fold] = true
		total += header.Size
		target := filepath.Join(destination, filepath.FromSlash(name))
		if directory {
			if err := os.MkdirAll(target, os.FileMode(header.Mode).Perm()); err != nil {
				return err
			}
			continue
		}
		if link {
			rel := strings.TrimPrefix(name, archiveRoot+"/")
			record, ok := fileRecord(manifest.Files, rel)
			if manifest.Platform != "macos" || !ok || record.LinkTarget != header.Linkname {
				return fmt.Errorf("unsafe archive link %q", header.Name)
			}
			links = append(links, pendingLink{target, header.Linkname})
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := copyExclusive(target, reader, os.FileMode(header.Mode).Perm()); err != nil {
			return err
		}
	}
	for _, link := range links {
		if err := os.MkdirAll(filepath.Dir(link.name), 0o755); err != nil {
			return err
		}
		if err := os.Symlink(link.target, link.name); err != nil {
			return err
		}
	}
	return nil
}

func copyExclusive(name string, reader io.Reader, mode os.FileMode) error {
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, io.LimitReader(reader, maxPackageFileBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(name, mode)
}

func VerifyTree(root string, manifest PackageManifest) error {
	actual := map[string]os.FileInfo{}
	err := filepath.Walk(root, func(name string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		if rel == "." || info.IsDir() {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if info.Mode()&os.ModeType != 0 && info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("package links and special files are forbidden: %s", rel)
		}
		actual[rel] = info
		return nil
	})
	if err != nil {
		return err
	}
	delete(actual, PackageManifestName)
	if len(actual) != len(manifest.Files) {
		return fmt.Errorf("package file count mismatch")
	}
	for _, record := range manifest.Files {
		info, ok := actual[record.Path]
		if !ok {
			return fmt.Errorf("missing package file %s", record.Path)
		}
		if record.LinkTarget != "" {
			if info.Mode()&os.ModeSymlink == 0 {
				return fmt.Errorf("framework link is not a symlink: %s", record.Path)
			}
			target, err := os.Readlink(filepath.Join(root, filepath.FromSlash(record.Path)))
			if err != nil || target != record.LinkTarget {
				return fmt.Errorf("framework link target mismatch: %s", record.Path)
			}
			continue
		}
		if !info.Mode().IsRegular() || info.Size() != record.Size {
			return fmt.Errorf("size mismatch for %s", record.Path)
		}
		if manifest.Platform != "windows" && fmt.Sprintf("%04o", info.Mode().Perm()) != record.Mode {
			return fmt.Errorf("mode mismatch for %s", record.Path)
		}
		file, err := os.Open(filepath.Join(root, filepath.FromSlash(record.Path)))
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, err = io.Copy(hash, file)
		file.Close()
		if err != nil {
			return err
		}
		if hex.EncodeToString(hash.Sum(nil)) != record.SHA256 {
			return fmt.Errorf("hash mismatch for %s", record.Path)
		}
	}
	for _, component := range []Component{manifest.Components.Application, manifest.Components.Server, manifest.Components.Launcher, manifest.Components.Manager} {
		if err := verifyExecutableArchitecture(filepath.Join(root, filepath.FromSlash(component.Path)), manifest.Platform, manifest.Architecture); err != nil {
			return err
		}
	}
	if manifest.Components.CredentialHelper != nil {
		if err := verifyExecutableArchitecture(filepath.Join(root, filepath.FromSlash(manifest.Components.CredentialHelper.Path)), manifest.Platform, manifest.Architecture); err != nil {
			return err
		}
	}
	// Desktop schema stays compatible with older managers; new managers also
	// validate the optional CLI executables before making them public entries.
	if manifest.PackageKind == "" {
		extension := ""
		if manifest.Platform == "windows" {
			extension = ".exe"
		}
		for _, name := range []string{PackageCLIPath(manifest.Platform, ""), "bootstrap/headroom-cli" + extension} {
			if _, exists := fileRecord(manifest.Files, name); exists {
				if err := verifyExecutableArchitecture(filepath.Join(root, filepath.FromSlash(name)), manifest.Platform, manifest.Architecture); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func verifyExecutableArchitecture(filename, platform, architecture string) error {
	if platform == "windows" {
		file, err := pe.Open(filename)
		if err != nil {
			return fmt.Errorf("inspect PE %s: %w", filepath.Base(filename), err)
		}
		defer file.Close()
		expected := uint16(pe.IMAGE_FILE_MACHINE_AMD64)
		if architecture == "arm64" {
			expected = pe.IMAGE_FILE_MACHINE_ARM64
		}
		if file.FileHeader.Machine != expected {
			return fmt.Errorf("wrong PE architecture for %s", filepath.Base(filename))
		}
		return nil
	}
	if platform == "macos" {
		expected := macho.CpuAmd64
		if architecture == "arm64" {
			expected = macho.CpuArm64
		}
		file, err := macho.Open(filename)
		if err == nil {
			defer file.Close()
			if file.Cpu != expected {
				return fmt.Errorf("wrong Mach-O architecture for %s", filepath.Base(filename))
			}
			return nil
		}
		fat, fatErr := macho.OpenFat(filename)
		if fatErr != nil {
			return fmt.Errorf("inspect Mach-O %s: %w", filepath.Base(filename), err)
		}
		defer fat.Close()
		for _, arch := range fat.Arches {
			if arch.Cpu == expected {
				return nil
			}
		}
		return fmt.Errorf("wrong Mach-O architecture for %s", filepath.Base(filename))
	}
	file, err := elf.Open(filename)
	if err != nil {
		return fmt.Errorf("inspect ELF %s: %w", filepath.Base(filename), err)
	}
	defer file.Close()
	expected := elf.EM_X86_64
	if architecture == "arm64" {
		expected = elf.EM_AARCH64
	}
	if file.Machine != expected {
		return fmt.Errorf("wrong ELF architecture for %s", filepath.Base(filename))
	}
	return nil
}

func WriteArchive(root, output string) error {
	base := filepath.Base(root)
	if strings.HasSuffix(output, ".zip") {
		file, err := os.Create(output)
		if err != nil {
			return err
		}
		writer := zip.NewWriter(file)
		walkErr := walkArchiveFiles(root, func(rel, name string, info os.FileInfo) error {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink forbidden in zip: %s", rel)
			}
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			header.Name = path.Join(base, filepath.ToSlash(rel))
			header.Method = zip.Deflate
			entry, err := writer.CreateHeader(header)
			if err != nil {
				return err
			}
			src, err := os.Open(name)
			if err != nil {
				return err
			}
			defer src.Close()
			_, err = io.Copy(entry, src)
			return err
		})
		closeErr := writer.Close()
		fileErr := file.Close()
		if walkErr != nil {
			return walkErr
		}
		if closeErr != nil {
			return closeErr
		}
		return fileErr
	}
	if strings.HasSuffix(output, ".tar.gz") {
		file, err := os.Create(output)
		if err != nil {
			return err
		}
		gz := gzip.NewWriter(file)
		gz.ModTime = gzip.Header{}.ModTime
		writer := tar.NewWriter(gz)
		walkErr := walkArchiveFiles(root, func(rel, name string, info os.FileInfo) error {
			link := ""
			if info.Mode()&os.ModeSymlink != 0 {
				var err error
				link, err = os.Readlink(name)
				if err != nil {
					return err
				}
			}
			header, err := tar.FileInfoHeader(info, link)
			if err != nil {
				return err
			}
			header.Name = path.Join(base, filepath.ToSlash(rel))
			header.ModTime = header.ModTime.UTC()
			if err = writer.WriteHeader(header); err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			src, err := os.Open(name)
			if err != nil {
				return err
			}
			defer src.Close()
			_, err = io.Copy(writer, src)
			return err
		})
		close1 := writer.Close()
		close2 := gz.Close()
		close3 := file.Close()
		if walkErr != nil {
			return walkErr
		}
		if close1 != nil {
			return close1
		}
		if close2 != nil {
			return close2
		}
		return close3
	}
	return fmt.Errorf("unsupported archive output")
}

func walkArchiveFiles(root string, visit func(string, string, os.FileInfo) error) error {
	var names []string
	err := filepath.Walk(root, func(name string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			names = append(names, name)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		info, err := os.Lstat(name)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, name)
		if err = visit(rel, name, info); err != nil {
			return err
		}
	}
	return nil
}

func FileDigest(name string) (int64, string, error) {
	file, err := os.Open(name)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, "", err
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return 0, "", err
	}
	return info.Size(), hex.EncodeToString(hash.Sum(nil)), nil
}
