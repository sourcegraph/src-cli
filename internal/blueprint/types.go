package blueprint

import "path/filepath"

// Blueprint represents a collection of Sourcegraph resources defined in a
// blueprint.yaml file.
type Blueprint struct {
	Dir         string         `yaml:"-"`
	Version     int            `yaml:"version"`
	Name        string         `yaml:"name"`
	Title       string         `yaml:"title"`
	Summary     string         `yaml:"summary"`
	Description string         `yaml:"description"`
	Category    string         `yaml:"category"`
	Tags        []string       `yaml:"tags"`
	BatchSpecs  []BatchSpecRef `yaml:"batchSpecs"`
	Monitors    []MonitorRef   `yaml:"monitors"`
	Insights    []InsightRef   `yaml:"insights"`
	Dashboards  []DashboardRef `yaml:"dashboards"`
}

// BatchSpecRef references a batch spec resource within a blueprint.
type BatchSpecRef struct {
	Name string `yaml:"name"`
}

// MonitorRef references a code monitor resource within a blueprint.
type MonitorRef struct {
	Name string `yaml:"name"`
}

// InsightRef references a code insight resource within a blueprint.
type InsightRef struct {
	Name       string   `yaml:"name"`
	Dashboards []string `yaml:"dashboards"`
}

// DashboardRef references an insights dashboard resource within a blueprint.
type DashboardRef struct {
	Name string `yaml:"name"`
}

// Path returns the filesystem path to the batch spec YAML for this reference.
func (r BatchSpecRef) Path(blueprintDir string) string {
	return filepath.Join(blueprintDir, "resources", "batch-spec", r.Name+".yaml")
}

// Path returns the filesystem path to the monitor GraphQL file for this reference.
func (r MonitorRef) Path(blueprintDir string) string {
	return filepath.Join(blueprintDir, "resources", "monitors", r.Name+".gql")
}

// Path returns the filesystem path to the insight GraphQL file for this reference.
func (r InsightRef) Path(blueprintDir string) string {
	return filepath.Join(blueprintDir, "resources", "insights", r.Name+".gql")
}

// Path returns the filesystem path to the dashboard GraphQL file for this reference.
func (r DashboardRef) Path(blueprintDir string) string {
	return filepath.Join(blueprintDir, "resources", "dashboards", r.Name+".gql")
}
