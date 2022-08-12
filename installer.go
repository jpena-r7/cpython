package cpython

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/paketo-buildpacks/packit/v2"
	"github.com/paketo-buildpacks/packit/v2/pexec"
	"github.com/paketo-buildpacks/packit/v2/postal"
	"github.com/paketo-buildpacks/packit/v2/scribe"
)

// postal.Dependency with extra field(s) specify to cpython
type CpythonDependency struct {
	postal.Dependency

	// Flags used for conigure before make and make install
	ConfigureFlags []string `toml:"configure_flags"`
}

// parse buildpack.toml and return CpythonDependency for the given version
func getCpythonDependency(path string, genericDependecy postal.Dependency) (CpythonDependency, error) {
	file, err := os.Open(path)
	if err != nil {
		return CpythonDependency{}, fmt.Errorf("failed to parse buildpack.toml: %w", err)
	}

	var buildpack struct {
		Metadata struct {
			DefaultVersions map[string]string   `toml:"default-versions"`
			Dependencies    []CpythonDependency `toml:"dependencies"`
		} `toml:"metadata"`
	}
	_, err = toml.NewDecoder(file).Decode(&buildpack)
	if err != nil {
		return CpythonDependency{}, fmt.Errorf("failed to parse buildpack.toml: %w", err)
	}

	genericDependecyStacks := strings.Join(genericDependecy.Stacks, " ")

	for _, dependency := range buildpack.Metadata.Dependencies {
		dependecyStacks := strings.Join(dependency.Stacks, " ")
		if dependency.Version == genericDependecy.Version && dependecyStacks == genericDependecyStacks {
			return dependency, nil
		}
	}
	return CpythonDependency{}, fmt.Errorf(
		"failed to find dependency for version %s and stack %s",
		genericDependecy.Version,
		genericDependecy.Stacks,
	)
}

func InstallPython(context packit.BuildContext, genericDependecy postal.Dependency, layer packit.Layer, logger scribe.Emitter) error {
	dependency, err := getCpythonDependency(
		filepath.Join(context.CNBPath, "buildpack.toml"), genericDependecy,
	)
	if err != nil {
		return err
	}

	logger.Debug.Subprocess("(CpythonDependecy) dependency: %+v\n", dependency)

	configureFlags := dependency.ConfigureFlags
	configureFlags = append(configureFlags, "--prefix="+layer.Path)
	sourcePath := filepath.Join(layer.Path, SourceName)

	commandEnv := []string{}

	if os.Chdir(sourcePath) != nil {
		return err
	}

	configure := pexec.NewExecutable(filepath.Join(sourcePath, "configure"))

	logger.Subprocess("Running 'configure %s'", strings.Join(configureFlags, " "))
	err = configure.Execute(pexec.Execution{
		Args:   configureFlags,
		Env:    commandEnv,
		Stdout: logger.Debug.ActionWriter,
		Stderr: logger.Debug.ActionWriter,
	})
	if err != nil {
		return err
	}

	make := pexec.NewExecutable("make")

	makeFlags := []string{"-j", fmt.Sprint(runtime.NumCPU()), `LDFLAGS="-Wl,--strip-all"`}
	logger.Subprocess("Running 'make %s'", strings.Join(makeFlags, " "))
	err = make.Execute(pexec.Execution{
		Args:   makeFlags,
		Env:    commandEnv,
		Stdout: logger.Debug.ActionWriter,
		Stderr: logger.Debug.ActionWriter,
	})
	if err != nil {
		return err
	}

	makeInstallFlags := []string{"altinstall"}
	logger.Subprocess("Running 'make %s'", strings.Join(makeInstallFlags, " "))
	err = make.Execute(pexec.Execution{
		Args:   makeInstallFlags,
		Env:    commandEnv,
		Stdout: logger.Debug.ActionWriter,
		Stderr: logger.Debug.ActionWriter,
	})
	if err != nil {
		return err
	}

	versionList := strings.Split(dependency.Version, ".")
	major := versionList[0]
	majorMinor := strings.Join(versionList[:len(versionList)-1], ".")

	if os.Chdir(filepath.Join(layer.Path, "bin")) != nil {
		return err
	}

	for _, name := range []string{"python", "pip", "pydoc"} {
		logger.Debug.Action("Writing symlink bin/%s", name+major)
		if err = os.Symlink(name+majorMinor, name+major); err != nil {
			return err
		}
	}

	if os.Chdir(context.WorkingDir) != nil {
		return err
	}

	return nil
}
