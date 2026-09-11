package contract

import "testing"

func TestPrivateWindowsDACLAllowsOnlyProtectedOwnerControl(t *testing.T) {
	owner := "S-1-5-21-100"
	for _, valid := range []string{
		"O:" + owner + "D:P(A;;FA;;;" + owner + ")",
		"O:" + owner + "D:PAI(A;;FA;;;" + owner + ")",
		"O:" + owner + "D:PARAI(A;;FA;;;" + owner + ")",
	} {
		if !privateWindowsDACLIsSafe(valid, owner) {
			t.Errorf("safe DACL rejected: %s", valid)
		}
	}
	for _, invalid := range []string{
		"O:" + owner + "D:(A;;FA;;;" + owner + ")",
		"O:" + owner + "D:P(A;;FR;;;" + owner + ")",
		"O:" + owner + "D:P(A;;FA;;;" + owner + ")(A;;FR;;;WD)",
		"O:" + owner + "D:PX(A;;FA;;;" + owner + ")",
	} {
		if privateWindowsDACLIsSafe(invalid, owner) {
			t.Errorf("unsafe DACL accepted: %s", invalid)
		}
	}
}
