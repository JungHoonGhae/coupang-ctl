package browserbridge

import "errors"

var ErrInstallationConflict = errors.New("browser bridge installation conflicts with existing or modified local state")
