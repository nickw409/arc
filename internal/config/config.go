package config

// Supported values for validation.
var (
	SupportedLanguages = []string{"rust", "go", "typescript", "python", "unknown"}
	SupportedRunners   = []string{"cargo-nextest", "cargo-test", "go-test", "vitest", "pytest", "unknown"}
	SupportedStyles    = []string{"conventional", "freeform"}
)

// Config represents the project-level .arc.yaml configuration.
type Config struct {
	Language       string    `yaml:"language"`
	Runner         string    `yaml:"runner"`
	DefaultPackage string    `yaml:"default_package"`
	BuildCommand   string    `yaml:"build_command"`
	TestCommand    string    `yaml:"test_command"`
	Git            GitConfig `yaml:"git"`
}

// GitConfig holds git-related settings.
type GitConfig struct {
	CommitStyle string `yaml:"commit_style"`
	Sign        bool   `yaml:"sign"`
	CoAuthor    bool   `yaml:"co_author"`
}

// Load reads .arc.yaml from the given project root.
func Load(projectRoot string) (*Config, error) {
	panic("not implemented")
}

// Validate checks that language is in SupportedLanguages and runner is in SupportedRunners.
func (c *Config) Validate() error {
	panic("not implemented")
}
