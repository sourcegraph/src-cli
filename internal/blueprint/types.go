package blueprint

import "path/filepath"

type Blueprint struct {
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

type BatchSpecRef struct {
	Name string `yaml:"name"`
}

type MonitorRef struct {
	Name string `yaml:"name"`
}

type InsightRef struct {
	Name       string   `yaml:"name"`
	Dashboards []string `yaml:"dashboards"`
}

type DashboardRef struct {
	Name string `yaml:"name"`
}

func (r BatchSpecRef) Path(blueprintDir string) string {
	return filepath.Join(blueprintDir, "resources", "batch-spec", r.Name+".yaml")
}

func (r MonitorRef) Path(blueprintDir string) string {
	return filepath.Join(blueprintDir, "resources", "monitors", r.Name+".gql")
}

func (r InsightRef) Path(blueprintDir string) string {
	return filepath.Join(blueprintDir, "resources", "insights", r.Name+".gql")
}

func (r DashboardRef) Path(blueprintDir string) string {
	return filepath.Join(blueprintDir, "resources", "dashboards", r.Name+".gql")
}
