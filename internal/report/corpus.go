package report

// CorpusRepository is one repository of the frozen evaluation corpus.
type CorpusRepository struct {
	Name   string
	Commit string
	Role   string
}

// Corpus roles.
const (
	RoleDevelopment     = "development"
	RoleTemplateControl = "template_control"
	RoleYoungControl    = "young_control"
)

// The frozen development and control repositories, in corpus-table order.
// The configuration holdout is deliberately absent: calibration never names
// it, so nothing that writes a lock can read it.
var (
	GolangTools = CorpusRepository{
		Name:   "golang/tools",
		Commit: "d68c5293798322055c6b3ec11cf34d1e8705438f",
		Role:   RoleDevelopment,
	}
	Kubernetes = CorpusRepository{
		Name:   "kubernetes/kubernetes",
		Commit: "a231bf3f37761e8955eefba61855da1a526a3eb9",
		Role:   RoleDevelopment,
	}
	GoStarter = CorpusRepository{
		Name:   "allaboutapps/go-starter",
		Commit: "9f7a54cca5cf7b68657e6d5242bab3fcef81ae2d",
		Role:   RoleTemplateControl,
	}
	Proficiency = CorpusRepository{
		Name:   "tuxerrante/proficiency",
		Commit: "f0a8a795013de170318e6a6b879fdcd0e45cd50e",
		Role:   RoleYoungControl,
	}
)

// CalibrationCorpus lists the development and control repositories in
// corpus-table order.
func CalibrationCorpus() []CorpusRepository {
	return []CorpusRepository{GolangTools, Kubernetes, GoStarter, Proficiency}
}

// CorpusRepositoryFor returns the calibration corpus entry whose frozen commit
// is commit.
func CorpusRepositoryFor(commit string) (CorpusRepository, bool) {
	for _, repository := range CalibrationCorpus() {
		if repository.Commit == commit {
			return repository, true
		}
	}

	return CorpusRepository{}, false
}
