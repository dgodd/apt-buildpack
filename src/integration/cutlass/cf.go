package cutlass

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tidwall/gjson"
)

type cfConfig struct {
	SpaceFields struct {
		GUID string
	}
}
type cfApps struct {
	Resources []struct {
		Metadata struct {
			GUID string `json:"guid"`
		} `json:"metadata"`
	} `json:"resources"`
}
type cfInstance struct {
	State string `json:"state"`
}

type App struct {
	Name      string
	Path      string
	Buildpack string
	Stdout    *bytes.Buffer
	appGUID   string
}

func New(fixture string) *App {
	return &App{
		Name:      filepath.Base(fixture),
		Path:      fixture,
		Buildpack: "",
		appGUID:   "",
	}
}

func (a *App) SpaceGUID() (string, error) {
	bytes, err := ioutil.ReadFile(filepath.Join(os.Getenv("HOME"), ".cf", "config.json"))
	if err != nil {
		return "", err
	}
	var config cfConfig
	if err := json.Unmarshal(bytes, &config); err != nil {
		return "", err
	}
	return config.SpaceFields.GUID, nil
}

func (a *App) AppGUID() (string, error) {
	if a.appGUID != "" {
		return a.appGUID, nil
	}
	guid, err := a.SpaceGUID()
	if err != nil {
		return "", err
	}
	cmd := exec.Command("cf", "curl", "/v2/apps?q=space_guid:"+guid+"&q=name:"+a.Name)
	bytes, err := cmd.Output()
	if err != nil {
		return "", err
	}
	var apps cfApps
	if err := json.Unmarshal(bytes, &apps); err != nil {
		return "", err
	}
	if len(apps.Resources) != 1 {
		return "", fmt.Errorf("Expected one app, found %d", len(apps.Resources))
	}
	a.appGUID = apps.Resources[0].Metadata.GUID
	return a.appGUID, nil
}

func (a *App) InstanceStates() ([]string, error) {
	guid, err := a.AppGUID()
	if err != nil {
		return []string{}, err
	}
	cmd := exec.Command("cf", "curl", "/v2/apps/"+guid+"/instances")
	bytes, err := cmd.Output()
	if err != nil {
		return []string{}, err
	}
	var data map[string]cfInstance
	if err := json.Unmarshal(bytes, &data); err != nil {
		return []string{}, err
	}
	var states []string
	for _, value := range data {
		states = append(states, value.State)
	}
	return states, nil
}

func (a *App) Push() error {
	args := []string{"push", a.Name, "--no-start", "--random-route", "-p", a.Path}
	if a.Buildpack != "" {
		args = append(args, "-b", a.Buildpack)
	}
	if _, err := os.Stat(filepath.Join(a.Path, "manifest.yml")); err == nil {
		args = append(args, "-f", filepath.Join(a.Path, "manifest.yml"))
	}
	command := exec.Command("cf", args...)
	if data, err := command.Output(); err != nil {
		fmt.Println(string(data))
		return err
	}

	command = exec.Command("cf", "logs", a.Name)
	a.Stdout = bytes.NewBuffer(nil)
	command.Stdout = a.Stdout
	if err := command.Start(); err != nil {
		return err
	}

	command = exec.Command("cf", "start", a.Name)
	if data, err := command.Output(); err != nil {
		fmt.Println(string(data))
		return err
	}
	return nil
}

func (a *App) GetUrl(path string) (string, error) {
	guid, err := a.AppGUID()
	if err != nil {
		return "", err
	}
	cmd := exec.Command("cf", "curl", "/v2/apps/"+guid+"/summary")
	data, err := cmd.Output()
	if err != nil {
		return "", err
	}
	host := gjson.Get(string(data), "routes.0.host").String()
	domain := gjson.Get(string(data), "routes.0.domain.name").String()
	return fmt.Sprintf("http://%s.%s%s", host, domain, path), nil
}

func (a *App) GetBody(path string) (string, error) {
	url, err := a.GetUrl(path)
	if err != nil {
		return "", err
	}
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	// TODO: Non 200 ??
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), err
}

func (a *App) HasFile(path string) error {
	cmd := exec.Command("cf", "ssh", a.Name, "-c", "ls -d "+path)
	return cmd.Run()
}

func (a *App) Destroy() error {
	command := exec.Command("cf", "delete", "-f", a.Name)
	return command.Run()
}
