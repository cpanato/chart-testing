// Copyright The Helm Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver"
	helmignore "helm.sh/helm/v3/pkg/ignore"

	"github.com/helm/chart-testing/v3/pkg/config"
	"github.com/helm/chart-testing/v3/pkg/exec"
	"github.com/helm/chart-testing/v3/pkg/ignore"
	"github.com/helm/chart-testing/v3/pkg/tool"
	"github.com/helm/chart-testing/v3/pkg/util"
)

const maxNameLength = 63

// Git is the Interface that wraps Git operations.
//
// FileExistsOnBranch checks whether file exists on the specified remote/branch.
//
// Show returns the contents of file on the specified remote/branch.
//
// AddWorktree checks out the contents of the repository at a commit ref into the specified path.
//
// RemoveWorktree removes the working tree at the specified path.
//
// MergeBase returns the SHA1 of the merge base of commit1 and commit2.
//
// ListChangedFilesInDirs diffs commit against HEAD and returns changed files for the specified dirs.
//
// GetURLForRemote returns the repo URL for the specified remote.
//
// ValidateRepository checks that the current working directory is a valid git repository,
// and returns nil if valid.
//
// BranchExists checks whether a given branch exists in the git repository.
type Git interface {
	FileExistsOnBranch(file string, remote string, branch string) bool
	Show(file string, remote string, branch string) (string, error)
	AddWorktree(path string, ref string) error
	RemoveWorktree(path string) error
	MergeBase(commit1 string, commit2 string) (string, error)
	ListChangedFilesInDirs(commit string, dirs ...string) ([]string, error)
	GetURLForRemote(remote string) (string, error)
	ValidateRepository() error
	BranchExists(branch string) bool
}

// Helm is the interface that wraps Helm operations
//
// # AddRepo adds a chart repository to the local Helm configuration
//
// # BuildDependencies builds the chart's dependencies
//
// # BuildDependenciesWithArgs allows passing additional arguments to BuildDependencies
//
// LintWithValues runs `helm lint` for the given chart using the specified values file.
// Pass a zero value for valuesFile in order to run lint without specifying a values file.
//
// InstallWithValues runs `helm install` for the given chart using the specified values file.
// Pass a zero value for valuesFile in order to run install without specifying a values file.
//
// UpgradeWithValues runs `helm upgrade` against an existing release using the specified values file.
// Pass a zero value for valuesFile in order to run install without specifying a values file.
//
// Test runs `helm test` against an existing release. Set the cleanup argument to true in order
// to clean up test pods created by helm after the test command completes.
//
// DeleteRelease purges the specified Helm release.
type Helm interface {
	AddRepo(name string, url string, extraArgs []string) error
	BuildDependencies(chart string) error
	BuildDependenciesWithArgs(chart string, extraArgs []string) error
	LintWithValues(chart string, valuesFile string) error
	InstallWithValues(chart string, valuesFile string, namespace string, release string) error
	UpgradeWithValues(chart string, valuesFile string, namespace string, release string) error
	Test(namespace string, release string) error
	DeleteRelease(namespace string, release string)
	Version() (string, error)
}

// Kubectl is the interface that wraps kubectl operations
//
// # DeleteNamespace deletes a namespace
//
// # WaitForDeployments waits for a deployment to become ready
//
// # GetPodsforDeployment gets all pods for a deployment
//
// # GetPods gets pods for the given args
//
// # GetEvents prints all events for namespace
//
// # DescribePod prints the pod's description
//
// # Logs prints the logs of container
//
// # GetInitContainers gets all init containers of pod
//
// GetContainers gets all containers of pod
type Kubectl interface {
	CreateNamespace(namespace string) error
	DeleteNamespace(namespace string)
	WaitForDeployments(namespace string, selector string) error
	GetPodsforDeployment(namespace string, deployment string) ([]string, error)
	GetPods(args ...string) ([]string, error)
	GetEvents(namespace string) error
	DescribePod(namespace string, pod string) error
	Logs(namespace string, pod string, container string) error
	GetInitContainers(namespace string, pod string) ([]string, error)
	GetContainers(namespace string, pod string) ([]string, error)
}

// Linter is the interface that wrap linting operations
//
// # YamlLint runs `yamllint` on the specified file with the specified configuration
//
// Yamale runs `yamale` on the specified file with the specified schema file
type Linter interface {
	YamlLint(yamlFile string, configFile string) error
	Yamale(yamlFile string, schemaFile string) error
}

// CmdExecutor is the interface
//
// RunCommand renders cmdTemplate as go template using data and executes the resulting command
type CmdExecutor interface {
	RunCommand(cmdTemplate string, data interface{}) error
}

// DirectoryLister is the interface
//
// ListChildDirs lists direct child directories of parentDir given they pass the test function
type DirectoryLister interface {
	ListChildDirs(parentDir string, test func(string) bool) ([]string, error)
}

// Utils is the interface that wraps chart-related methods
//
// LookupChartDir looks up the chart's root directory based on some chart file that has changed
type Utils interface {
	LookupChartDir(chartDirs []string, dir string) (string, error)
}

// AccountValidator is the interface that wraps Git account validation
//
// Validate checks if account is valid on repoDomain
type AccountValidator interface {
	Validate(repoDomain string, account string) error
}

// Chart represents a Helm chart, and can be initialized with the NewChart method.
type Chart struct {
	path          string
	yaml          *util.ChartYaml
	ciValuesPaths []string
}

// Yaml returns the Chart metadata
func (c *Chart) Yaml() *util.ChartYaml {
	return c.yaml
}

// Path returns the chart's directory path
func (c *Chart) Path() string {
	return c.path
}

func (c *Chart) String() string {
	return fmt.Sprintf(`%s => (version: "%s", path: "%s")`, c.yaml.Name, c.yaml.Version, c.Path())
}

// ValuesFilePathsForCI returns all file paths in the 'ci' subfolder of the chart directory matching the pattern '*-values.yaml'
func (c *Chart) ValuesFilePathsForCI() []string {
	return c.ciValuesPaths
}

// HasCIValuesFile checks whether a given CI values file is present.
func (c *Chart) HasCIValuesFile(path string) bool {
	fileName := filepath.Base(path)
	for _, file := range c.ValuesFilePathsForCI() {
		if fileName == filepath.Base(file) {
			return true
		}
	}
	return false
}

// CreateInstallParams generates a randomized release name and namespace based on the chart path
// and optional buildID. If release_name is specified, the release name is set to that string instead.
// If a buildID is specified, it will be part of the generated namespace.
func (c *Chart) CreateInstallParams(buildID string, releaseName string) (release string, namespace string) {
	release = filepath.Base(c.Path())
	if release == "." || release == "/" {
		if releaseName != "" {
			release = releaseName
		} else {
			yaml := c.Yaml()
			release = yaml.Name
		}
	}
	namespace = release
	if buildID != "" {
		namespace = fmt.Sprintf("%s-%s", namespace, buildID)
	}
	randomSuffix := util.RandomString(10)
	release = util.SanitizeName(fmt.Sprintf("%s-%s", release, randomSuffix), maxNameLength)
	namespace = util.SanitizeName(fmt.Sprintf("%s-%s", namespace, randomSuffix), maxNameLength)
	return
}

// NewChart parses the path to a chart directory and allocates a new Chart object. If chartPath is
// not a valid chart directory an error is returned.
func NewChart(chartPath string) (*Chart, error) {
	yaml, err := util.ReadChartYaml(chartPath)
	if err != nil {
		return nil, err
	}
	matches, _ := filepath.Glob(filepath.Join(chartPath, "ci", "*-values.yaml"))
	return &Chart{chartPath, yaml, matches}, nil
}

type Testing struct {
	config                   config.Configuration
	helm                     Helm
	kubectl                  Kubectl
	git                      Git
	linter                   Linter
	cmdExecutor              CmdExecutor
	accountValidator         AccountValidator
	directoryLister          DirectoryLister
	utils                    Utils
	previousRevisionWorktree string
	loadRules                func(string) (*helmignore.Rules, error)
}

// TestResults holds results and overall status
type TestResults struct {
	OverallSuccess bool
	TestResults    []TestResult
}

// TestResult holds test results for a specific chart
type TestResult struct {
	Chart *Chart
	Error error
}

// NewTesting creates a new Testing struct with the given config.
func NewTesting(config config.Configuration, extraSetArgs string) (Testing, error) {
	procExec := exec.NewProcessExecutor(config.Debug)
	helmExtraArgs := strings.Fields(config.HelmExtraArgs)
	helmLintExtraArgs := strings.Fields(config.HelmLintExtraArgs)

	testing := Testing{
		config:           config,
		helm:             tool.NewHelm(procExec, helmExtraArgs, helmLintExtraArgs, strings.Fields(extraSetArgs)),
		git:              tool.NewGit(procExec),
		kubectl:          tool.NewKubectl(procExec, config.KubectlTimeout),
		linter:           tool.NewLinter(procExec),
		cmdExecutor:      tool.NewCmdTemplateExecutor(procExec),
		accountValidator: tool.AccountValidator{},
		directoryLister:  util.DirectoryLister{},
		utils:            util.Utils{},
		loadRules:        ignore.LoadRules,
	}

	versionString, err := testing.helm.Version()
	if err != nil {
		return testing, err
	}

	version, err := semver.NewVersion(versionString)
	if err != nil {
		return testing, err
	}

	if version.Major() < 3 {
		return testing, fmt.Errorf("minimum required Helm version is v3.0.0; found: %s", version)
	}
	return testing, nil
}

// computePreviousRevisionPath converts any file or directory path to the same path in the
// previous revision's working tree.
func (t *Testing) computePreviousRevisionPath(fileOrDirPath string) string {
	return filepath.Join(t.previousRevisionWorktree, fileOrDirPath)
}

func (t *Testing) processCharts(action func(chart *Chart) TestResult) ([]TestResult, error) {
	var results []TestResult // nolint: prealloc
	chartDirs, err := t.FindChartDirsToBeProcessed()
	if err != nil {
		return nil, fmt.Errorf("failed identifying charts to process: %w", err)
	} else if len(chartDirs) == 0 {
		return results, nil
	}

	var charts []*Chart
	for _, dir := range chartDirs {
		chart, err := NewChart(dir)
		if err != nil {
			return nil, err
		}

		if t.config.ExcludeDeprecated && chart.yaml.Deprecated {
			fmt.Printf("Chart %q is deprecated and will be ignored because '--exclude-deprecated' is set\n", chart.String())
		} else {
			charts = append(charts, chart)
		}
	}

	if !t.config.GithubGroups {
		fmt.Println()
		util.PrintDelimiterLineToWriter(os.Stdout, "-")
		fmt.Println(" Charts to be processed:")
		util.PrintDelimiterLineToWriter(os.Stdout, "-")
	} else {
		util.GithubGroupsBegin(os.Stdout, "Charts to be processed")
	}
	for _, chart := range charts {
		fmt.Printf(" %s\n", chart)
	}
	if !t.config.GithubGroups {
		util.PrintDelimiterLineToWriter(os.Stdout, "-")
		fmt.Println()
	} else {
		util.GithubGroupsEnd(os.Stdout)
	}

	repoArgs := map[string][]string{}

	for _, repo := range t.config.HelmRepoExtraArgs {
		repoSlice := strings.SplitN(repo, "=", 2)
		name := repoSlice[0]
		repoExtraArgs := strings.Fields(repoSlice[1])
		repoArgs[name] = repoExtraArgs
	}

	for _, repo := range t.config.ChartRepos {
		repoSlice := strings.SplitN(repo, "=", 2)
		name := repoSlice[0]
		url := repoSlice[1]

		repoExtraArgs := repoArgs[name]
		if err := t.helm.AddRepo(name, url, repoExtraArgs); err != nil {
			return nil, fmt.Errorf("failed adding repo: %s=%s: %w", name, url, err)
		}
	}

	testResults := TestResults{
		OverallSuccess: true,
		TestResults:    results,
	}

	// Checkout previous chart revisions and build their dependencies
	if t.config.Upgrade {
		mergeBase, err := t.computeMergeBase()
		if err != nil {
			return results, fmt.Errorf("failed identifying merge base: %w", err)
		}
		// Add worktree for the target revision
		worktreePath, err := os.MkdirTemp("./", "ct-previous-revision")
		if err != nil {
			return results, fmt.Errorf("could not create previous revision directory: %w", err)
		}
		t.previousRevisionWorktree = worktreePath
		err = t.git.AddWorktree(worktreePath, mergeBase)
		if err != nil {
			return results, fmt.Errorf("could not create worktree for previous revision: %w", err)
		}
		defer t.git.RemoveWorktree(worktreePath) // nolint: errcheck

		if !t.config.SkipHelmDependencies {
			for _, chart := range charts {
				if err := t.helm.BuildDependenciesWithArgs(t.computePreviousRevisionPath(chart.Path()), t.config.HelmDependencyExtraArgs); err != nil {
					// Only print error (don't exit) if building dependencies for previous revision fails.
					fmt.Printf("failed building dependencies for previous revision of chart %q: %v\n", chart, err.Error())
				}
			}
		}
	}

	for _, chart := range charts {
		if !t.config.SkipHelmDependencies {
			if err := t.helm.BuildDependenciesWithArgs(chart.Path(), t.config.HelmDependencyExtraArgs); err != nil {
				return nil, fmt.Errorf("failed building dependencies for chart %q: %w", chart, err)
			}
		}

		result := action(chart)
		if result.Error != nil {
			testResults.OverallSuccess = false
		}
		results = append(results, result)
	}
	if testResults.OverallSuccess {
		return results, nil
	}

	return results, fmt.Errorf("failed processing charts")
}

// LintCharts lints charts (changed, all, specific) depending on the configuration.
func (t *Testing) LintCharts() ([]TestResult, error) {
	return t.processCharts(t.LintChart)
}

// InstallCharts install charts (changed, all, specific) depending on the configuration.
func (t *Testing) InstallCharts() ([]TestResult, error) {
	return t.processCharts(t.InstallChart)
}

// LintAndInstallCharts first lints and then installs charts (changed, all, specific) depending on the configuration.
func (t *Testing) LintAndInstallCharts() ([]TestResult, error) {
	return t.processCharts(t.LintAndInstallChart)
}

// PrintResults writes test results to stdout.
func (t *Testing) PrintResults(results []TestResult) {
	if !t.config.GithubGroups {
		fmt.Println()
		util.PrintDelimiterLineToWriter(os.Stdout, "-")
	} else {
		util.GithubGroupsBegin(os.Stdout, "Test Results")
	}
	if results != nil {
		for _, result := range results {
			err := result.Error
			if err != nil {
				fmt.Printf(" %s %s > %s\n", "✖︎", result.Chart, err)
			} else {
				fmt.Printf(" %s %s\n", "✔︎", result.Chart)
			}
		}
	} else {
		fmt.Println("No chart changes detected.")
	}
	if !t.config.GithubGroups {
		util.PrintDelimiterLineToWriter(os.Stdout, "-")
	} else {
		util.GithubGroupsEnd(os.Stdout)
	}
}

// LintChart lints the specified chart.
func (t *Testing) LintChart(chart *Chart) TestResult {
	fmt.Printf("Linting chart %q\n", chart)

	result := TestResult{Chart: chart}

	if t.config.CheckVersionIncrement {
		if err := t.CheckVersionIncrement(chart); err != nil {
			result.Error = err
			return result
		}
	}

	chartYaml := filepath.Join(chart.Path(), "Chart.yaml")
	valuesYaml := filepath.Join(chart.Path(), "values.yaml")
	valuesFiles := chart.ValuesFilePathsForCI()

	if t.config.ValidateChartSchema {
		if err := t.linter.Yamale(chartYaml, t.config.ChartYamlSchema); err != nil {
			result.Error = err
			return result
		}
	}

	if t.config.ValidateYaml {
		yamlFiles := append([]string{chartYaml, valuesYaml}, valuesFiles...)
		for _, yamlFile := range yamlFiles {
			if err := t.linter.YamlLint(yamlFile, t.config.LintConf); err != nil {
				result.Error = err
				return result
			}
		}
	}

	if t.config.ValidateMaintainers {
		if err := t.ValidateMaintainers(chart); err != nil {
			result.Error = err
			return result
		}
	}

	for _, cmd := range t.config.AdditionalCommands {
		if err := t.cmdExecutor.RunCommand(cmd, chart); err != nil {
			result.Error = err
			return result
		}
	}

	// Lint with defaults if no values files are specified.
	if len(valuesFiles) == 0 {
		valuesFiles = append(valuesFiles, "")
	}

	for _, valuesFile := range valuesFiles {
		if valuesFile != "" {
			fmt.Printf("\nLinting chart with values file %q...\n\n", valuesFile)
		}
		if err := t.helm.LintWithValues(chart.Path(), valuesFile); err != nil {
			result.Error = err
			break
		}
	}

	return result
}

// InstallChart installs the specified chart into a new namespace, waits for resources to become ready, and eventually
// uninstalls it and deletes the namespace again.
func (t *Testing) InstallChart(chart *Chart) TestResult {
	var result TestResult

	if t.config.Upgrade {
		// Test upgrade from previous version
		result = t.UpgradeChart(chart)
		if result.Error != nil {
			return result
		}
		// Test upgrade of current version (related: https://github.com/helm/chart-testing/issues/19)
		if err := t.doUpgrade(chart, chart, true); err != nil {
			result.Error = err
			return result
		}
	}

	result = TestResult{Chart: chart}
	if err := t.doInstall(chart); err != nil {
		result.Error = err
	}

	return result
}

// UpgradeChart tests in-place upgrades of the specified chart relative to its previous revisions. If the
// initial install or helm test of a previous revision of the chart fails, that release is ignored and no
// error will be returned. If the latest revision of the chart introduces a potentially breaking change
// according to the SemVer specification, upgrade testing will be skipped.
func (t *Testing) UpgradeChart(chart *Chart) TestResult {
	result := TestResult{Chart: chart}

	breakingChangeAllowed, err := t.checkBreakingChangeAllowed(chart)

	if breakingChangeAllowed {
		if err != nil {
			fmt.Printf("Skipping upgrade test of %q because: %v\n", chart, err.Error())
		}
		return result
	} else if err != nil {
		fmt.Printf("Error comparing chart versions for %q\n", chart)
		result.Error = err
		return result
	}

	if oldChart, err := NewChart(t.computePreviousRevisionPath(chart.Path())); err == nil {
		result.Error = t.doUpgrade(oldChart, chart, false)
	}

	return result
}

func (t *Testing) doInstall(chart *Chart) error {
	fmt.Printf("Installing chart %q...\n", chart)
	valuesFiles := chart.ValuesFilePathsForCI()

	// Test with defaults if no values files are specified.
	if len(valuesFiles) == 0 {
		valuesFiles = append(valuesFiles, "")
	}

	for _, valuesFile := range valuesFiles {
		if valuesFile != "" {
			fmt.Printf("\nInstalling chart with values file %q...\n\n", valuesFile)
		}

		// Use anonymous function. Otherwise deferred calls would pile up
		// and be executed in reverse order after the loop.
		fun := func() error {
			namespace, release, releaseSelector, cleanup := t.generateInstallConfig(chart)
			if !t.config.SkipCleanUp {
				defer cleanup()
			}

			if t.config.Namespace == "" {
				if err := t.kubectl.CreateNamespace(namespace); err != nil {
					return err
				}
			}
			if err := t.helm.InstallWithValues(chart.Path(), valuesFile, namespace, release); err != nil {
				return err
			}
			return t.testRelease(namespace, release, releaseSelector)
		}

		if err := fun(); err != nil {
			return err
		}
	}

	return nil
}

func (t *Testing) doUpgrade(oldChart, newChart *Chart, oldChartMustPass bool) error {
	fmt.Printf("Testing upgrades of chart %q relative to previous revision %q...\n", newChart, oldChart)
	valuesFiles := oldChart.ValuesFilePathsForCI()
	if len(valuesFiles) == 0 {
		valuesFiles = append(valuesFiles, "")
	}
	for _, valuesFile := range valuesFiles {
		if valuesFile != "" {
			if t.config.SkipMissingValues && !newChart.HasCIValuesFile(valuesFile) {
				fmt.Printf("Upgrade testing for values file %q skipped because a corresponding values file was not found in %s/ci\n", valuesFile, newChart.Path())
				continue
			}
			fmt.Printf("\nInstalling chart %q with values file %q...\n\n", oldChart, valuesFile)
		}

		// Use anonymous function. Otherwise deferred calls would pile up
		// and be executed in reverse order after the loop.
		fun := func() error {
			namespace, release, releaseSelector, cleanup := t.generateInstallConfig(oldChart)
			if !t.config.SkipCleanUp {
				defer cleanup()
			}

			if t.config.Namespace == "" {
				if err := t.kubectl.CreateNamespace(namespace); err != nil {
					return err
				}
			}
			// Install previous version of chart. If installation fails, ignore this release.
			if err := t.helm.InstallWithValues(oldChart.Path(), valuesFile, namespace, release); err != nil {
				if oldChartMustPass {
					return err
				}
				fmt.Printf("Upgrade testing for release %q skipped because of previous revision installation error: %v\n", release, err.Error())
				return nil
			}
			if err := t.testRelease(namespace, release, releaseSelector); err != nil {
				if oldChartMustPass {
					return err
				}
				fmt.Printf("Upgrade testing for release %q skipped because of previous revision testing error: %v\n", release, err.Error())
				return nil
			}

			if err := t.helm.UpgradeWithValues(newChart.Path(), valuesFile, namespace, release); err != nil {
				return err
			}

			return t.testRelease(namespace, release, releaseSelector)
		}

		if err := fun(); err != nil {
			return err
		}
	}

	return nil
}

func (t *Testing) testRelease(namespace, release, releaseSelector string) error {
	if err := t.kubectl.WaitForDeployments(namespace, releaseSelector); err != nil {
		return err
	}

	return t.helm.Test(namespace, release)
}

func (t *Testing) generateInstallConfig(chart *Chart) (namespace, release, releaseSelector string, cleanup func()) {
	if t.config.Namespace != "" {
		namespace = t.config.Namespace
		release, _ = chart.CreateInstallParams(t.config.BuildID, t.config.ReleaseName)
		releaseSelector = fmt.Sprintf("%s=%s", t.config.ReleaseLabel, release)
		cleanup = func() {
			t.PrintEventsPodDetailsAndLogs(namespace, releaseSelector)
			t.helm.DeleteRelease(namespace, release)
		}
	} else {
		release, namespace = chart.CreateInstallParams(t.config.BuildID, t.config.ReleaseName)
		cleanup = func() {
			t.PrintEventsPodDetailsAndLogs(namespace, releaseSelector)
			t.helm.DeleteRelease(namespace, release)
			t.kubectl.DeleteNamespace(namespace)
		}
	}

	return
}

// LintAndInstallChart first lints and then installs the specified chart.
func (t *Testing) LintAndInstallChart(chart *Chart) TestResult {
	result := t.LintChart(chart)
	if result.Error != nil {
		return result
	}
	return t.InstallChart(chart)
}

// FindChartDirsToBeProcessed identifies charts to be processed depending on the configuration
// (changed charts, all charts, or specific charts).
func (t *Testing) FindChartDirsToBeProcessed() ([]string, error) {
	cfg := t.config
	if cfg.ProcessAllCharts {
		return t.ReadAllChartDirectories()
	} else if len(cfg.Charts) > 0 {
		return t.config.Charts, nil
	}
	return t.ComputeChangedChartDirectories()
}

func (t *Testing) computeMergeBase() (string, error) {
	err := t.git.ValidateRepository()
	if err != nil {
		return "", errors.New("must be in a git repository")
	}

	branch := fmt.Sprintf("%s/%s", t.config.Remote, t.config.TargetBranch)
	if !t.git.BranchExists(branch) {
		return "", fmt.Errorf("targetBranch '%s' does not exist", branch)
	}

	return t.git.MergeBase(branch, t.config.Since)
}

// ComputeChangedChartDirectories takes the merge base of HEAD and the configured remote and target branch and computes a
// slice of changed charts from that in the configured chart directories excluding those configured to be excluded.
func (t *Testing) ComputeChangedChartDirectories() ([]string, error) {
	cfg := t.config

	mergeBase, err := t.computeMergeBase()
	if err != nil {
		return nil, err
	}

	allChangedChartFiles, err := t.git.ListChangedFilesInDirs(mergeBase, cfg.ChartDirs...)
	if err != nil {
		return nil, fmt.Errorf("failed creating diff: %w", err)
	}

	changedChartFiles := map[string][]string{}
	for _, file := range allChangedChartFiles {
		pathElements := strings.SplitN(filepath.ToSlash(file), "/", 3)
		if len(pathElements) < 2 || util.StringSliceContains(cfg.ExcludedCharts, pathElements[1]) {
			continue
		}
		dir := filepath.Dir(file)
		// Make sure directory is really a chart directory
		chartDir, err := t.utils.LookupChartDir(cfg.ChartDirs, dir)
		chartDirElement := strings.Split(chartDir, "/")
		if err == nil {
			if len(chartDirElement) > 1 {
				chartDirName := chartDirElement[len(chartDirElement)-1]
				if util.StringSliceContains(cfg.ExcludedCharts, chartDirName) {
					continue
				}
			}
			changedChartFiles[chartDir] = append(changedChartFiles[chartDir], strings.TrimPrefix(file, chartDir+"/"))
		} else {
			fmt.Fprintf(os.Stderr, "Directory %q is not a valid chart directory. Skipping...\n", dir)
		}
	}

	changedChartDirs := []string{}
	if t.config.UseHelmignore {
		for chartDir, changedChartFiles := range changedChartFiles {
			rules, err := t.loadRules(chartDir)
			if err != nil {
				return nil, err
			}
			filteredChartFiles, err := ignore.FilterFiles(changedChartFiles, rules)
			if err != nil {
				return nil, err
			}
			if len(filteredChartFiles) > 0 {
				changedChartDirs = append(changedChartDirs, chartDir)
			}
		}
	} else {
		for chartDir := range changedChartFiles {
			changedChartDirs = append(changedChartDirs, chartDir)
		}
	}

	return changedChartDirs, nil
}

// ReadAllChartDirectories returns a slice of all charts in the configured chart directories except those
// configured to be excluded.
func (t *Testing) ReadAllChartDirectories() ([]string, error) {
	cfg := t.config

	var chartDirs []string
	for _, chartParentDir := range cfg.ChartDirs {
		dirs, err := t.directoryLister.ListChildDirs(chartParentDir,
			func(dir string) bool {
				_, err := t.utils.LookupChartDir(cfg.ChartDirs, dir)
				return err == nil && !util.StringSliceContains(cfg.ExcludedCharts, filepath.Base(dir))
			})
		if err != nil {
			return nil, fmt.Errorf("failed reading chart directories: %w", err)
		}
		chartDirs = append(chartDirs, dirs...)
	}

	return chartDirs, nil
}

// CheckVersionIncrement checks that the new chart version is greater than the old one using semantic version comparison.
func (t *Testing) CheckVersionIncrement(chart *Chart) error {
	fmt.Printf("Checking chart %q for a version bump...\n", chart)

	oldVersion, err := t.GetOldChartVersion(chart.Path())
	if err != nil {
		return err
	}
	if oldVersion == "" {
		// new chart, skip version check
		return nil
	}

	fmt.Println("Old chart version:", oldVersion)

	chartYaml := chart.Yaml()
	newVersion := chartYaml.Version
	fmt.Println("New chart version:", newVersion)

	result, err := util.CompareVersions(oldVersion, newVersion)
	if err != nil {
		return err
	}

	if result >= 0 {
		return errors.New("chart version not ok. Needs a version bump! ")
	}

	fmt.Println("Chart version ok.")
	return nil
}

func (t *Testing) checkBreakingChangeAllowed(chart *Chart) (allowed bool, err error) {
	oldVersion, err := t.GetOldChartVersion(chart.Path())
	if err != nil {
		return false, err
	}
	if oldVersion == "" {
		// new chart, skip upgrade check
		return true, fmt.Errorf("chart has no previous revision")
	}

	newVersion := chart.Yaml().Version

	return util.BreakingChangeAllowed(oldVersion, newVersion)
}

// GetOldChartVersion gets the version of the old Chart.yaml file from the target branch.
func (t *Testing) GetOldChartVersion(chartPath string) (string, error) {
	cfg := t.config

	chartYamlFile := filepath.Join(chartPath, "Chart.yaml")
	if !t.git.FileExistsOnBranch(chartYamlFile, cfg.Remote, cfg.TargetBranch) {
		fmt.Printf("Unable to find chart on %s. New chart detected.\n", cfg.TargetBranch)
		return "", nil
	}

	chartYamlContents, err := t.git.Show(chartYamlFile, cfg.Remote, cfg.TargetBranch)
	if err != nil {
		return "", fmt.Errorf("failed reading old Chart.yaml: %w", err)
	}

	chartYaml, err := util.UnmarshalChartYaml([]byte(chartYamlContents))
	if err != nil {
		return "", fmt.Errorf("failed reading old chart version: %w", err)
	}

	return chartYaml.Version, nil
}

// ValidateMaintainers validates maintainers in the Chart.yaml file. Maintainer names must be valid accounts
// (GitHub, Bitbucket, GitLab) names. Deprecated charts must not have maintainers.
func (t *Testing) ValidateMaintainers(chart *Chart) error {
	fmt.Println("Validating maintainers...")

	chartYaml := chart.Yaml()

	if chartYaml.Deprecated {
		if len(chartYaml.Maintainers) > 0 {
			return errors.New("deprecated chart must not have maintainers")
		}
		return nil
	}

	if len(chartYaml.Maintainers) == 0 {
		return errors.New("chart doesn't have maintainers")
	}

	repoURL, err := t.git.GetURLForRemote(t.config.Remote)
	if err != nil {
		return err
	}

	for _, maintainer := range chartYaml.Maintainers {
		if err := t.accountValidator.Validate(repoURL, maintainer.Name); err != nil {
			return err
		}
	}

	return nil
}

func (t *Testing) PrintEventsPodDetailsAndLogs(namespace string, selector string) {
	util.PrintDelimiterLineToWriter(os.Stdout, "=")

	t.printDetails(namespace, "Events of namespace", ".", func(_ string) error {
		return t.kubectl.GetEvents(namespace)
	}, namespace)

	pods, err := t.kubectl.GetPods(
		"--no-headers",
		"--namespace",
		namespace,
		"--selector",
		selector,
		"--output",
		"jsonpath={.items[*].metadata.name}",
	)
	if err != nil {
		fmt.Println("Error printing logs:", err)
		return
	}

	for _, pod := range pods {
		t.printDetails(pod, "Description of pod", "~", func(_ string) error {
			return t.kubectl.DescribePod(namespace, pod)
		}, pod)

		initContainers, err := t.kubectl.GetInitContainers(namespace, pod)
		if err != nil {
			fmt.Println("Error printing logs:", err)
			return
		}

		if t.config.PrintLogs {
			t.printDetails(pod, "Logs of init container", "-",
				func(item string) error {
					return t.kubectl.Logs(namespace, pod, item)
				}, initContainers...)

			containers, err := t.kubectl.GetContainers(namespace, pod)
			if err != nil {
				fmt.Printf("failed printing logs: %v\n", err.Error())
				return
			}

			t.printDetails(pod, "Logs of container", "-",
				func(item string) error {
					return t.kubectl.Logs(namespace, pod, item)
				},
				containers...)
		}
	}

	util.PrintDelimiterLineToWriter(os.Stdout, "=")
}

func (t *Testing) printDetails(resource string, text string, delimiterChar string, printFunc func(item string) error, items ...string) {
	for _, item := range items {
		item = strings.Trim(item, "'")

		if !t.config.GithubGroups {
			util.PrintDelimiterLineToWriter(os.Stdout, delimiterChar)
			fmt.Printf("==> %s %s\n", text, resource)
			util.PrintDelimiterLineToWriter(os.Stdout, delimiterChar)
		} else {
			util.GithubGroupsBegin(os.Stdout, fmt.Sprintf("%s %s", text, resource))
		}

		if err := printFunc(item); err != nil {
			fmt.Println("Error printing details:", err)
			return
		}

		if !t.config.GithubGroups {
			util.PrintDelimiterLineToWriter(os.Stdout, delimiterChar)
			fmt.Printf("<== %s %s\n", text, resource)
			util.PrintDelimiterLineToWriter(os.Stdout, delimiterChar)
		} else {
			util.GithubGroupsEnd(os.Stdout)
		}
	}
}
