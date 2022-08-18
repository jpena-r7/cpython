package cpython

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/paketo-buildpacks/packit/v2"
	"github.com/paketo-buildpacks/packit/v2/pexec"
	"github.com/paketo-buildpacks/packit/v2/postal"
	"github.com/paketo-buildpacks/packit/v2/scribe"
)

func InstallPython(sourcePath string, context packit.BuildContext, entry packit.BuildpackPlanEntry, dependency postal.Dependency, layer packit.Layer, logger scribe.Emitter) error {
	flags, _ := entry.Metadata["configure-flags"].(string)

	if flags == "" {
		flags = "--enable-optimizations --with-ensurepip"
		logger.Debug.Subprocess("Using default configure flags: %v\n", flags)
	}

	whiteSpace := regexp.MustCompile(`\s+`)
	configureFlags := whiteSpace.Split(flags, -1)
	configureFlags = append(configureFlags, "--prefix="+layer.Path)

	commandEnv := []string{}

	if err := os.Chdir(sourcePath); err != nil {
		return err
	}

	configure := pexec.NewExecutable(filepath.Join(sourcePath, "configure"))

	logger.Debug.Subprocess("Running 'configure %s'", strings.Join(configureFlags, " "))
	err := configure.Execute(pexec.Execution{
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
	logger.Debug.Subprocess("Running 'make %s'", strings.Join(makeFlags, " "))
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
	logger.Debug.Subprocess("Running 'make %s'", strings.Join(makeInstallFlags, " "))
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

	if err = os.Chdir(filepath.Join(layer.Path, "bin")); err != nil {
		return err
	}

	for _, name := range []string{"python", "pip", "pydoc"} {
		logger.Debug.Action("Writing symlink bin/%s", name+major)
		if err = os.Symlink(name+majorMinor, name+major); err != nil {
			return err
		}
	}

	if err = os.Chdir(context.WorkingDir); err != nil {
		return err
	}

	return nil
}
