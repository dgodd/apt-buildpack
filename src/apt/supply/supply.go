package supply

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cloudfoundry/libbuildpack"
)

type Command interface {
	Execute(string, io.Writer, io.Writer, string, ...string) error
}

type Stager interface {
	// AddBinDependencyLink(string, string) error
	BuildDir() string
	CacheDir() string
	DepDir() string
	DepsIdx() string
	WriteEnvFile(string, string) error
	WriteProfileD(string, string) error
	WriteConfigYml(interface{}) error
}

type Supplier struct {
	Stager      Stager
	Command     Command
	Log         *libbuildpack.Logger
	AptCacheDir string
	AptStateDir string
}

func New(stager Stager, command Command, logger *libbuildpack.Logger) *Supplier {
	return &Supplier{
		Stager:      stager,
		Command:     command,
		Log:         logger,
		AptCacheDir: filepath.Join(stager.CacheDir(), "apt", "cache"),
		AptStateDir: filepath.Join(stager.CacheDir(), "apt", "state"),
	}
}

func Run(s *Supplier) error {
	if err := s.InstallApt(); err != nil {
		s.Log.Error("Error installing packages: %s", err.Error())
		return err
	}

	if err := s.ConfigureFinalizeEnv(); err != nil {
		s.Log.Error("Error writing environment vars: %s", err.Error())
		return nil
	}

	if err := s.Stager.WriteConfigYml(nil); err != nil {
		s.Log.Error("Error writing config.yml: %s", err.Error())
		return err
	}

	return nil
}

func (s *Supplier) InstallApt() error {
	if err := os.Setenv("APT_CACHE_DIR", s.AptCacheDir); err != nil {
		s.Log.Error("Error setting env: %s", err.Error())
		return err
	}
	if err := os.MkdirAll(s.AptCacheDir, 0755); err != nil {
		s.Log.Error("Error creating directory: %s", err.Error())
		return err
	}
	if err := os.Setenv("APT_STATE_DIR", s.AptStateDir); err != nil {
		s.Log.Error("Error setting env: %s", err.Error())
		return err
	}
	if err := os.MkdirAll(s.AptStateDir, 0755); err != nil {
		s.Log.Error("Error creating directory: %s", err.Error())
		return err
	}

	s.Log.BeginStep("Updating apt caches")
	buffer := new(bytes.Buffer)
	if err := s.Command.Execute("", buffer, ioutil.Discard, "apt-get", "-o", "debug::nolocking=true", "-o", "dir::cache="+s.AptCacheDir, "-o", "dir::state="+s.AptStateDir, "update"); err != nil {
		s.Log.Error("Updating apt cache failed")
		s.Log.Info(strings.TrimSpace(buffer.String()))
		return err
	}

	var aptfile []string
	if err := libbuildpack.NewYAML().Load(filepath.Join(s.Stager.BuildDir(), "Aptfile"), &aptfile); err != nil {
		s.Log.Error("Could not read Aptfile: %s", err.Error())
		return err
	}

	for _, pkg := range aptfile {
		if strings.HasSuffix(pkg, ".deb") {
			name := path.Base(pkg)
			file := filepath.Join(s.AptCacheDir, "archives", name)
			s.Log.BeginStep("Fetching " + pkg)
			if err := s.downloadFile(pkg, file); err != nil {
				s.Log.Error("Could not download package")
				return err
			}
		} else {
			s.Log.BeginStep("Fetching .debs for " + pkg)
			buffer := new(bytes.Buffer)
			if err := s.Command.Execute("", buffer, buffer,
				"apt-get",
				"-o", "debug::nolocking=true",
				"-o", "dir::cache="+s.AptCacheDir,
				"-o", "dir::state="+s.AptStateDir,
				"-y", "--force-yes", "-d",
				"install", "--reinstall", pkg,
			); err != nil {
				s.Log.Error("Could not download package")
				s.Log.Info(strings.TrimSpace(buffer.String()))
				return err
			}
		}
	}

	dirs, err := ioutil.ReadDir(filepath.Join(s.AptCacheDir, "archives"))
	if err != nil {
		s.Log.Error("Could not read archive")
		return err
	}
	for _, deb := range dirs {
		if strings.HasSuffix(deb.Name(), ".deb") {
			s.Log.BeginStep("Installing " + deb.Name())
			buffer := new(bytes.Buffer)
			if err := s.Command.Execute(filepath.Join(s.AptCacheDir, "archives"), buffer, buffer, "dpkg", "-x", deb.Name(), s.Stager.DepDir()); err != nil {
				s.Log.Error("Could not download package: %s", err.Error())
				s.Log.Info(strings.TrimSpace(buffer.String()))
				return err
			}
		}
	}

	return nil
}

func (s *Supplier) ConfigureFinalizeEnv() error {
	s.Log.BeginStep("Writing profile script")

	depDir := "$DEPS_DIR/" + s.Stager.DepsIdx()
	include_path := fmt.Sprintf("%s/usr/include:$INCLUDE_PATH", depDir)
	envs := [][]string{
		[]string{"PATH", fmt.Sprintf("%s/usr/bin:$PATH", depDir)},
		[]string{"LD_LIBRARY_PATH", fmt.Sprintf("%s/lib/x86_64-linux-gnu/:%s/usr/lib/x86_64-linux-gnu:%s/usr/lib/i386-linux-gnu:%s/usr/lib:$LD_LIBRARY_PATH", depDir, depDir, depDir, depDir)},
		[]string{"LIBRARY_PATH", fmt.Sprintf("%s/lib/x86_64-linux-gnu/:%s/usr/lib/x86_64-linux-gnu:%s/usr/lib/i386-linux-gnu:%s/usr/lib:$LIBRARY_PATH", depDir, depDir, depDir, depDir)},
		[]string{"INCLUDE_PATH", include_path},
		[]string{"CPATH", include_path},
		[]string{"CPPPATH", include_path},
		[]string{"PKG_CONFIG_PATH", fmt.Sprintf("%s/usr/lib/x86_64-linux-gnu/pkgconfig:%s/usr/lib/i386-linux-gnu/pkgconfig:%s/usr/lib/pkgconfig:$PKG_CONFIG_PATH", depDir, depDir, depDir)},
	}
	profileD := ""
	for _, env := range envs {
		if err := s.Stager.WriteEnvFile(env[0], env[1]); err != nil {
			return err
		}

		profileD = profileD + fmt.Sprintf(`export %s="%s"`, env[0], env[1]) + "\n"
	}

	if err := s.Stager.WriteProfileD("apt.sh", profileD); err != nil {
		return err
	}

	return nil
}

func (s *Supplier) downloadFile(url, destFile string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		s.Log.Error("Could not download: %d", resp.StatusCode)
		return errors.New("file download failed")
	}

	return s.writeToFile(resp.Body, destFile, 0666)
}

func (s *Supplier) writeToFile(source io.Reader, destFile string, mode os.FileMode) error {
	err := os.MkdirAll(filepath.Dir(destFile), 0755)
	if err != nil {
		s.Log.Error("Could not create %s", filepath.Dir(destFile))
		return err
	}

	fh, err := os.OpenFile(destFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		s.Log.Error("Could not create %s", destFile)
		return err
	}
	defer fh.Close()

	_, err = io.Copy(fh, source)
	if err != nil {
		s.Log.Error("Could not write to %s", destFile)
		return err
	}

	return nil
}
