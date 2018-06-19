package yaml

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/go-ozzo/ozzo-validation"
	"github.com/pkg/errors"
	"github.com/runatlantis/atlantis/server/events/yaml/raw"
	"github.com/runatlantis/atlantis/server/events/yaml/valid"
	"gopkg.in/yaml.v2"
)

// AtlantisYAMLFilename is the name of the config file for each repo.
const AtlantisYAMLFilename = "atlantis.yaml"

type ParserValidator struct{}

// ReadConfig returns the parsed and validated atlantis.yaml config for repoDir.
// If there was no config file, then this can be detected by checking the type
// of error: os.IsNotExist(error).
func (p *ParserValidator) ReadConfig(repoDir string) (valid.Spec, error) {
	configFile := filepath.Join(repoDir, AtlantisYAMLFilename)
	configData, err := ioutil.ReadFile(configFile)

	// NOTE: the error we return here must also be os.IsNotExist since that's
	// what our callers use to detect a missing config file.
	if err != nil && os.IsNotExist(err) {
		return valid.Spec{}, err
	}

	// If it exists but we couldn't read it return an error.
	if err != nil {
		return valid.Spec{}, errors.Wrapf(err, "unable to read %s file", AtlantisYAMLFilename)
	}

	// If the config file exists, parse it.
	config, err := p.parseAndValidate(configData)
	if err != nil {
		return valid.Spec{}, errors.Wrapf(err, "parsing %s", AtlantisYAMLFilename)
	}
	return config, err
}

func (p *ParserValidator) parseAndValidate(configData []byte) (valid.Spec, error) {
	var rawSpec raw.Spec
	if err := yaml.UnmarshalStrict(configData, &rawSpec); err != nil {
		return valid.Spec{}, err
	}

	// Set ErrorTag to yaml so it uses the YAML field names in error messages.
	validation.ErrorTag = "yaml"

	if err := rawSpec.Validate(); err != nil {
		return valid.Spec{}, err
	}

	// Top level validation.
	if err := p.validateWorkflows(rawSpec); err != nil {
		return valid.Spec{}, err
	}

	validSpec := rawSpec.ToValid()
	if err := p.validateProjectNames(validSpec); err != nil {
		return valid.Spec{}, err
	}

	return validSpec, nil
}

func (p *ParserValidator) validateProjectNames(spec valid.Spec) error {
	// First, validate that all names are unique.
	seen := make(map[string]bool)
	for _, project := range spec.Projects {
		if project.Name != nil {
			name := *project.Name
			exists := seen[name]
			if exists {
				return fmt.Errorf("found two or more projects with name %q; project names must be unique", name)
			}
			seen[name] = true
		}
	}

	// Next, validate that all dir/workspace combos are named.
	// This map's keys will be 'dir/workspace' and the values are the names for
	// that project.
	dirWorkspaceToNames := make(map[string][]string)
	for _, project := range spec.Projects {
		key := fmt.Sprintf("%s/%s", project.Dir, project.Workspace)
		names := dirWorkspaceToNames[key]

		// If there is already a project with this dir/workspace then this
		// project must have a name.
		if len(names) > 0 && project.Name == nil {
			return fmt.Errorf("there are two or more projects with dir: %q workspace: %q that are not all named; they must have a 'name' key so they can be targeted for apply's separately", project.Dir, project.Workspace)
		}
		var name string
		if project.Name != nil {
			name = *project.Name
		}
		dirWorkspaceToNames[key] = append(dirWorkspaceToNames[key], name)
	}

	return nil
}

func (p *ParserValidator) validateWorkflows(spec raw.Spec) error {
	for _, project := range spec.Projects {
		if err := p.validateWorkflowExists(project, spec.Workflows); err != nil {
			return err
		}
	}
	return nil
}

func (p *ParserValidator) validateWorkflowExists(project raw.Project, workflows map[string]raw.Workflow) error {
	if project.Workflow == nil {
		return nil
	}
	workflow := *project.Workflow
	for k := range workflows {
		if k == workflow {
			return nil
		}
	}
	return fmt.Errorf("workflow %q is not defined", workflow)
}
