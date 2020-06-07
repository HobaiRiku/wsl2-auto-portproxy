package config

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	"log"
	"os/user"
	"path"
	"regexp"
	"strconv"
	"strings"
	"wsl2-auto-portproxy/lib/util"
)

type Config struct {
	OnlyPredefined bool
	Predefined     PredefinedPorts
	Ignore         IgnorePorts
}

type PredefinedPorts struct {
	Tcp []PortProxy `json:"tcp"`
	Udp []PortProxy `json:"udp"`
}

type PortProxy struct {
	Local  int64
	Remote int64
}

func (pp PortProxy) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("$d:$d", pp.Local, pp.Remote)), nil
}

func (pp *PortProxy) UnmarshalJSON(data []byte) error {
	var ppStr string
	err := json.Unmarshal(data, &ppStr)
	if err != nil {
		return err
	}
	ppPorts := strings.Split(ppStr, ":")
	pp.Local, err = strconv.ParseInt(ppPorts[0], 10, 64)
	if err != nil {
		return err
	}
	pp.Remote, err = strconv.ParseInt(ppPorts[1], 10, 64)
	if err != nil {
		return err
	}
	return nil
}

type IgnorePorts struct {
	Tcp []int64 `json:"tcp"`
	Udp []int64 `json:"udp"`
}

// JsonFile is a struct to unmarshal config file
type JsonFile struct {
	OnlyPredefined bool            `json:"onlyPredefined"`
	Predefined     PredefinedPorts `json:"predefined"`
	Ignore         IgnorePorts     `json:"ignore"`
}

var jsonCommentRegexp = regexp.MustCompile(`/\*([\s\S]*?)\*/`)

func init() {
	// create config dir
	userHome, _ := user.Current()
	_, err := util.CreatePathIfNotExist(path.Join(userHome.HomeDir, ".wslpp"))
	if err != nil {
		log.Fatalf("config init error: %s", err)
	}
}

// GetConfig return the config object read from %HOMEPATH%/.wslpp/config.json
func GetConfig() (Config, error) {
	var out Config
	userHome, _ := user.Current()
	configFilePath := path.Join(userHome.HomeDir, ".wslpp/config.json")
	exists, _ := util.PathExists(configFilePath)
	if !exists {
		out.OnlyPredefined = false
		return out, nil
	}
	b, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		return out, errors.Wrap(err, "read file error")
	}
	b = jsonCommentRegexp.ReplaceAll(b, []byte{})
	if err = json.Unmarshal(b, &out); err != nil {
		return out, errors.Wrap(err, "unmarshal config json error")
	}
	return out, nil
}
