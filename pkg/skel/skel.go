// Copyright 2014-2016 CNI authors
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

// Package skel provides skeleton code for a CNI plugin.
// In particular, it implements argument parsing and validation.
package skel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/utils"
	"github.com/containernetworking/cni/pkg/version"
)

// CmdArgs captures all the arguments passed in to the plugin
// via both env vars and stdin
type CmdArgs struct {
	ContainerID string
	Netns       string
	IfName      string
	Args        string
	Path        string
	StdinData   []byte
}

type dispatcher struct {
	/*获取环境变量函数*/
	Getenv func(string) string
	/*标准输入*/
	Stdin  io.Reader
	/*标准输出*/
	Stdout io.Writer
	/*标准错误输出*/
	Stderr io.Writer

	ConfVersionDecoder version.ConfigDecoder
	VersionReconciler  version.Reconciler
}

type reqForCmdEntry map[string]bool

/*自环境变量中加载内容，返回cmd,cmdargs*/
func (t *dispatcher) getCmdArgsFromEnv() (string, *CmdArgs, *types.Error) {
	var cmd, contID, netns, ifName, args, path string

	vars := []struct {
		name      string /*环境变量名称*/
		val       *string/*环境变量要填充的变量指针*/
		reqForCmd reqForCmdEntry
	}{
		{
			"CNI_COMMAND",
			&cmd,
			reqForCmdEntry{
				"ADD":   true,
				"CHECK": true,
				"DEL":   true,
			},
		},
		{
			"CNI_CONTAINERID",
			&contID,
			reqForCmdEntry{
				"ADD":   true,
				"CHECK": true,
				"DEL":   true,
			},
		},
		{
			"CNI_NETNS",
			&netns,
			reqForCmdEntry{
				"ADD":   true,
				"CHECK": true,
				"DEL":   false,
			},
		},
		{
			"CNI_IFNAME",
			&ifName,
			reqForCmdEntry{
				"ADD":   true,
				"CHECK": true,
				"DEL":   true,
			},
		},
		{
			"CNI_ARGS",
			&args,
			reqForCmdEntry{
				"ADD":   false,
				"CHECK": false,
				"DEL":   false,
			},
		},
		{
			"CNI_PATH",
			&path,
			reqForCmdEntry{
				"ADD":   true,
				"CHECK": true,
				"DEL":   true,
			},
		},
	}

	argsMissing := make([]string, 0)
	for _, v := range vars {
		/*取v.name规定的环境变量的值，并设置v.value*/
		*v.val = t.Getenv(v.name)
		if *v.val == "" {
			/*此环境变量未配置，如果cmd不为空,则添加进argsMissing中*/
			if v.reqForCmd[cmd] || v.name == "CNI_COMMAND" {
				argsMissing = append(argsMissing, v.name)
			}
		}
	}

	if len(argsMissing) > 0 {
		/*存在多个未配置的环境变量，异常*/
		joined := strings.Join(argsMissing, ",")
		return "", nil, types.NewError(types.ErrInvalidEnvironmentVariables, fmt.Sprintf("required env variables [%s] missing", joined), "")
	}

	if cmd == "VERSION" {
		t.Stdin = bytes.NewReader(nil)
	}

	/*读取标准输入*/
	stdinData, err := ioutil.ReadAll(t.Stdin)
	if err != nil {
		return "", nil, types.NewError(types.ErrIOFailure, fmt.Sprintf("error reading from stdin: %v", err), "")
	}

	/*利用环境变量构造cmdArgs*/
	cmdArgs := &CmdArgs{
		ContainerID: contID,
		Netns:       netns,
		IfName:      ifName,
		Args:        args,
		Path:        path,
		StdinData:   stdinData,
	}
	return cmd, cmdArgs, nil
}

func (t *dispatcher) checkVersionAndCall(cmdArgs *CmdArgs, pluginVersionInfo version.PluginInfo, toCall/*检查通过后，要执行的回调*/ func(*CmdArgs) error) *types.Error {
	/*取cni版本号*/
	configVersion, err := t.ConfVersionDecoder.Decode(cmdArgs.StdinData)
	if err != nil {
		return types.NewError(types.ErrDecodingFailure, err.Error(), "")
	}
	
	/*检查版本是否支持*/
	verErr := t.VersionReconciler.Check(configVersion, pluginVersionInfo)
	if verErr != nil {
		return types.NewError(types.ErrIncompatibleCNIVersion, "incompatible CNI versions", verErr.Details())
	}

	/*触发toCall回调*/
	if err = toCall(cmdArgs); err != nil {
		if e, ok := err.(*types.Error); ok {
			// don't wrap Error in Error
			return e
		}
		return types.NewError(types.ErrInternal, err.Error(), "")
	}

	return nil
}

func validateConfig(jsonBytes []byte) *types.Error {
	var conf struct {
		Name string `json:"name"`
	}
	/*解析json成conf对象*/
	if err := json.Unmarshal(jsonBytes, &conf); err != nil {
		return types.NewError(types.ErrDecodingFailure, fmt.Sprintf("error unmarshall network config: %v", err), "")
	}
	if conf.Name == "" {
		/*name不能为空*/
		return types.NewError(types.ErrInvalidNetworkConfig, "missing network name", "")
	}
	
	/*校验name*/
	if err := utils.ValidateNetworkName(conf.Name); err != nil {
		return err
	}
	return nil
}

/*dispatcher类对外提供的pluginMain函数
 * 如果版本检查通过：
	Cmd=add时，cmdAdd被调用
    cmd=check时，cmdCheck被调用
    cmd=del时，cmdDel被调用
*/
func (t *dispatcher) pluginMain(cmdAdd, cmdCheck, cmdDel func(_ *CmdArgs) error, versionInfo version.PluginInfo, about string) *types.Error {
	/*自环境变量中提取cmd,cmdArgs*/
	cmd, cmdArgs, err := t.getCmdArgsFromEnv()
	if err != nil {
		// Print the about string to stderr when no command is set
		if err.Code == types.ErrInvalidEnvironmentVariables && t.Getenv("CNI_COMMAND") == "" && about != "" {
			_, _ = fmt.Fprintln(t.Stderr, about)
			_, _ = fmt.Fprintf(t.Stderr, "CNI protocol versions supported: %s\n", strings.Join(versionInfo.SupportedVersions(), ", "))
			return nil
		}
		return err
	}

	/*cmd = version时cmdArgs.stdinData为空*/
	if cmd != "VERSION" {
		/*校验stdinData对应的配置*/
		if err = validateConfig(cmdArgs.StdinData); err != nil {
			return err
		}
		
		/*校验containerID*/
		if err = utils.ValidateContainerID(cmdArgs.ContainerID); err != nil {
			return err
		}
		
		/*校验ifName*/
		if err = utils.ValidateInterfaceName(cmdArgs.IfName); err != nil {
			return err
		}
	}

	switch cmd {
	case "ADD":
		/*检查版本后，触发cmdAdd回调*/
		err = t.checkVersionAndCall(cmdArgs, versionInfo, cmdAdd)
	case "CHECK":
		/*取配置版本*/
		configVersion, err := t.ConfVersionDecoder.Decode(cmdArgs.StdinData)
		if err != nil {
			return types.NewError(types.ErrDecodingFailure, err.Error(), "")
		}
		/*和0.4.0进行版本比较*/
		if gtet, err := version.GreaterThanOrEqualTo(configVersion, "0.4.0"); err != nil {
			return types.NewError(types.ErrDecodingFailure, err.Error(), "")
		} else if !gtet {
			/*小于0.4.0*/
			return types.NewError(types.ErrIncompatibleCNIVersion, "config version does not allow CHECK", "")
		}
		
		/*遍历当前支持的版本，如果pluginVersion大于configVersion，则进行cmdCheck调用*/
		for _, pluginVersion := range versionInfo.SupportedVersions() {
			gtet, err := version.GreaterThanOrEqualTo(pluginVersion, configVersion)
			if err != nil {
				return types.NewError(types.ErrDecodingFailure, err.Error(), "")
			} else if gtet {
				/*触发cmdCheck调用*/
				if err := t.checkVersionAndCall(cmdArgs, versionInfo, cmdCheck); err != nil {
					return err
				}
				return nil
			}
		}
		return types.NewError(types.ErrIncompatibleCNIVersion, "plugin version does not allow CHECK", "")
	case "DEL":
		/*触发cmdDel回调*/
		err = t.checkVersionAndCall(cmdArgs, versionInfo, cmdDel)
	case "VERSION":
		/*仅进行version解析*/
		if err := versionInfo.Encode(t.Stdout); err != nil {
			return types.NewError(types.ErrIOFailure, err.Error(), "")
		}
	default:
		/*其它不认识的命令*/
		return types.NewError(types.ErrInvalidEnvironmentVariables, fmt.Sprintf("unknown CNI_COMMAND: %v", cmd), "")
	}

	return err
}

// PluginMainWithError is the core "main" for a plugin. It accepts
// callback functions for add, check, and del CNI commands and returns an error.
//
// The caller must also specify what CNI spec versions the plugin supports.
//
// It is the responsibility of the caller to check for non-nil error return.
//
// For a plugin to comply with the CNI spec, it must print any error to stdout
// as JSON and then exit with nonzero status code.
//
// To let this package automatically handle errors and call os.Exit(1) for you,
// use PluginMain() instead.
func PluginMainWithError(cmdAdd, cmdCheck, cmdDel func(_ *CmdArgs) error, versionInfo version.PluginInfo, about string) *types.Error {
	return (&dispatcher{
		Getenv: os.Getenv,/*通过os包提取环境变量*/
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}).pluginMain(cmdAdd, cmdCheck, cmdDel, versionInfo, about)
}

// PluginMain is the core "main" for a plugin which includes automatic error handling.
//
// The caller must also specify what CNI spec versions the plugin supports.
//
// The caller can specify an "about" string, which is printed on stderr
// when no CNI_COMMAND is specified. The recommended output is "CNI plugin <foo> v<version>"
//
// When an error occurs in either cmdAdd, cmdCheck, or cmdDel, PluginMain will print the error
// as JSON to stdout and call os.Exit(1).
//
// To have more control over error handling, use PluginMainWithError() instead.
func PluginMain(cmdAdd, cmdCheck, cmdDel func(_ *CmdArgs) error, versionInfo version.PluginInfo, about string) {
	/*自env中获取cmd，并按cmd要求执行cmdAdd/cmdCheck/cmdDel*/
	if e := PluginMainWithError(cmdAdd, cmdCheck, cmdDel, versionInfo, about); e != nil {
		if err := e.Print(); err != nil {
			log.Print("Error writing error JSON to stdout: ", err)
		}
		os.Exit(1)
	}
}
