package browserbridge

type platformRegistration interface {
	Preflight(manifestPath string) error
	Install(manifestPath string) error
	Check(manifestPath string) error
	Uninstall(manifestPath string) error
}

type filePlatformRegistration struct{}

func (filePlatformRegistration) Preflight(string) error { return nil }
func (filePlatformRegistration) Install(string) error   { return nil }
func (filePlatformRegistration) Check(string) error     { return nil }
func (filePlatformRegistration) Uninstall(string) error { return nil }
