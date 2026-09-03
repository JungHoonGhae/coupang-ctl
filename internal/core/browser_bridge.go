package core

const BrowserBridgeSchemaVersion = 1

type BrowserBridgeInstallation struct {
	SchemaVersion          int    `json:"schema_version"`
	Status                 string `json:"status"`
	Browser                string `json:"browser"`
	ExtensionID            string `json:"extension_id"`
	ExtensionPath          string `json:"extension_path"`
	NativeHostName         string `json:"native_host_name"`
	NativeHostManifestPath string `json:"native_host_manifest_path"`
	InstallationRecordPath string `json:"installation_record_path"`
	NextAction             string `json:"next_action,omitempty"`
}

type BrowserBridgeDoctorReport struct {
	SchemaVersion          int     `json:"schema_version"`
	Status                 string  `json:"status"`
	Ready                  bool    `json:"ready"`
	Browser                string  `json:"browser"`
	ExtensionID            string  `json:"extension_id"`
	ExtensionPath          string  `json:"extension_path"`
	NativeHostName         string  `json:"native_host_name"`
	NativeHostManifestPath string  `json:"native_host_manifest_path"`
	InstallationRecordPath string  `json:"installation_record_path"`
	ExtensionLoadStatus    string  `json:"extension_load_status"`
	NextAction             string  `json:"next_action,omitempty"`
	Checks                 []Check `json:"checks"`
}

type BrowserBridgeUninstallResult struct {
	SchemaVersion int      `json:"schema_version"`
	Status        string   `json:"status"`
	Removed       []string `json:"removed"`
}
