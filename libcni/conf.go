// Copyright 2015 CNI authors
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

package libcni

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
)

type NotFoundError struct {
	Dir  string
	Name string
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf(`no net configuration with name "%s" in %s`, e.Name, e.Dir)
}

type NoConfigsFoundError struct {
	Dir string
}

func (e NoConfigsFoundError) Error() string {
	return fmt.Sprintf(`no net configurations found in %s`, e.Dir)
}

func ConfFromBytes(bytes []byte) (*NetworkConfig, error) {
	conf := &NetworkConfig{Bytes: bytes}
	if err := json.Unmarshal(bytes, &conf.Network); err != nil {
		return nil, fmt.Errorf("error parsing configuration: %w", err)
	}
	if conf.Network.Type == "" {
		return nil, fmt.Errorf("error parsing configuration: missing 'type'")
	}
	return conf, nil
}

func ConfFromFile(filename string) (*NetworkConfig, error) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %w", filename, err)
	}
	return ConfFromBytes(bytes)
}

/*依据配置文件，建立NetworkConfigList*/
func ConfListFromBytes(bytes []byte) (*NetworkConfigList, error) {
	/*转换bytes,先解析为key,value集合*/
	rawList := make(map[string]interface{})
	if err := json.Unmarshal(bytes, &rawList); err != nil {
		return nil, fmt.Errorf("error parsing configuration list: %w", err)
	}

	/*取name配置*/
	rawName, ok := rawList["name"]
	if !ok {
		return nil, fmt.Errorf("error parsing configuration list: no name")
	}
	name, ok := rawName.(string)
	if !ok {
		return nil, fmt.Errorf("error parsing configuration list: invalid name type %T", rawName)
	}

	/*取cni版本*/
	var cniVersion string
	rawVersion, ok := rawList["cniVersion"]
	if ok {
		cniVersion, ok = rawVersion.(string)
		if !ok {
			return nil, fmt.Errorf("error parsing configuration list: invalid cniVersion type %T", rawVersion)
		}
	}

	disableCheck := false
	if rawDisableCheck, ok := rawList["disableCheck"]; ok {
		disableCheck, ok = rawDisableCheck.(bool)
		if !ok {
			return nil, fmt.Errorf("error parsing configuration list: invalid disableCheck type %T", rawDisableCheck)
		}
	}

	/*构造list配置对象*/
	list := &NetworkConfigList{
		Name:         name,
		DisableCheck: disableCheck,
		CNIVersion:   cniVersion,
		Bytes:        bytes,
	}

	/*取plugins配置*/
	var plugins []interface{}
	plug, ok := rawList["plugins"]
	if !ok {
		return nil, fmt.Errorf("error parsing configuration list: no 'plugins' key")
	}
	plugins, ok = plug.([]interface{})
	if !ok {
		return nil, fmt.Errorf("error parsing configuration list: invalid 'plugins' type %T", plug)
	}
	if len(plugins) == 0 {
		/*plugins配置不得为空*/
		return nil, fmt.Errorf("error parsing configuration list: no plugins in list")
	}

	/*依据相应配置重女目土之netConf*/
	for i, conf := range plugins {
		newBytes, err := json.Marshal(conf)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal plugin config %d: %w", i, err)
		}
		netConf, err := ConfFromBytes(newBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse plugin config %d: %w", i, err)
		}
		list.Plugins = append(list.Plugins, netConf)
	}

	return list, nil
}

/*加载并解析配置文件到NetWorkConfigList*/
func ConfListFromFile(filename string) (*NetworkConfigList, error) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %w", filename, err)
	}
	return ConfListFromBytes(bytes)
}

/*收集满足后续的配置文件列表*/
func ConfFiles(dir string, extensions []string) ([]string, error) {
	// In part, adapted from rkt/networking/podenv.go#listFiles
	files, err := ioutil.ReadDir(dir)
	switch {
	case err == nil: // break
	case os.IsNotExist(err):
		return nil, nil
	default:
		return nil, err
	}

	confFiles := []string{}
	for _, f := range files {
		if f.IsDir() {
			/*跳过目录*/
			continue
		}
		fileExt := filepath.Ext(f.Name())
		for _, ext := range extensions {
			if fileExt == ext {
				/*f文件名称与对应的后缀匹配，记录在confFiles集合中*/
				confFiles = append(confFiles, filepath.Join(dir, f.Name()))
			}
		}
	}
	return confFiles, nil
}

/*加载.conf,.json文件，查找conf.Network.Name与所给参数配置的配置*/
func LoadConf(dir, name string) (*NetworkConfig, error) {
	files, err := ConfFiles(dir, []string{".conf", ".json"})
	switch {
	case err != nil:
		return nil, err
	case len(files) == 0:
		return nil, NoConfigsFoundError{Dir: dir}
	}
	sort.Strings(files)

	for _, confFile := range files {
		conf, err := ConfFromFile(confFile)
		if err != nil {
			return nil, err
		}
		if conf.Network.Name == name {
			return conf, nil
		}
	}
	return nil, NotFoundError{dir, name}
}

/*在指定目录加载配置文件*/
func LoadConfList(dir, name string) (*NetworkConfigList, error) {
	files, err := ConfFiles(dir, []string{".conflist"})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	/*按顺序加载配置文件*/
	for _, confFile := range files {
		/*加载配置文件*/
		conf, err := ConfListFromFile(confFile)
		if err != nil {
			return nil, err
		}
		/*仅返回conf.Name与参数匹配的配置*/
		if conf.Name == name {
			return conf, nil
		}
	}

	// Try and load a network configuration file (instead of list)
	// from the same name, then upconvert.
	singleConf, err := LoadConf(dir, name)
	if err != nil {
		// A little extra logic so the error makes sense
		if _, ok := err.(NoConfigsFoundError); len(files) != 0 && ok {
			// Config lists found but no config files found
			return nil, NotFoundError{dir, name}
		}

		return nil, err
	}
	/*利用单个conf,构造ConfList*/
	return ConfListFromConf(singleConf)
}

func InjectConf(original *NetworkConfig, newValues map[string]interface{}) (*NetworkConfig, error) {
	config := make(map[string]interface{})
	/*由字节串，打包成config对象*/
	err := json.Unmarshal(original.Bytes, &config)
	if err != nil {
		return nil, fmt.Errorf("unmarshal existing network bytes: %w", err)
	}

	/*利用newValues更新config中的配置*/
	for key, value := range newValues {
		if key == "" {
			return nil, fmt.Errorf("keys cannot be empty")
		}

		if value == nil {
			return nil, fmt.Errorf("key '%s' value must not be nil", key)
		}

		config[key] = value
	}

	/*再由对象打包成字节串（这一步单纯是为了校验？）*/
	newBytes, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	/*由字节串再打包成对象。*/
	return ConfFromBytes(newBytes)
}

// ConfListFromConf "upconverts" a network config in to a NetworkConfigList,
// with the single network as the only entry in the list.
func ConfListFromConf(original *NetworkConfig) (*NetworkConfigList, error) {
	// Re-deserialize the config's json, then make a raw map configlist.
	// This may seem a bit strange, but it's to make the Bytes fields
	// actually make sense. Otherwise, the generated json is littered with
	// golang default values.

	rawConfig := make(map[string]interface{})
	if err := json.Unmarshal(original.Bytes, &rawConfig); err != nil {
		return nil, err
	}

	rawConfigList := map[string]interface{}{
		"name":       original.Network.Name,
		"cniVersion": original.Network.CNIVersion,
		"plugins":    []interface{}{rawConfig},
	}

	b, err := json.Marshal(rawConfigList)
	if err != nil {
		return nil, err
	}
	return ConfListFromBytes(b)
}
