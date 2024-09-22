// Copyright 2016 CNI authors
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

package invoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/containernetworking/cni/pkg/types"
)

type RawExec struct {
	/*仅需要指定stderr*/
	Stderr io.Writer
}

/*RawExec对象通过pluginPath直接运行插件，并向其提供输入的json串及环境变量，返回其*/
func (e *RawExec) ExecPlugin(ctx context.Context, pluginPath string/*插件路径*/, stdinData []byte/*输入的json串*/, environ []string) ([]byte, error) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	c := exec.CommandContext(ctx, pluginPath)
	/*指明环境变量*/
	c.Env = environ
	/*向标准输入提供config*/
	c.Stdin = bytes.NewBuffer(stdinData)
	c.Stdout = stdout
	c.Stderr = stderr

	// Retry the command on "text file busy" errors
	for i := 0; i <= 5; i++ {
		/*执行此插件*/
		err := c.Run()

		// Command succeeded
		if err == nil {
			break
		}

		// If the plugin is currently about to be written, then we wait a
		// second and try it again
		if strings.Contains(err.Error(), "text file busy") {
			time.Sleep(time.Second)
			continue
		}

		// All other errors except than the busy text file
		return nil, e.pluginErr(err, stdout.Bytes(), stderr.Bytes())/*返回错误输出结果*/
	}

	// Copy stderr to caller's buffer in case plugin printed to both
	// stdout and stderr for some reason. Ignore failures as stderr is
	// only informational.
	if e.Stderr != nil && stderr.Len() > 0 {
		_, _ = stderr.WriteTo(e.Stderr)
	}
	
	/*返回执行结果,执行成功（这里有问题吧，5次之后就成功了？）*/
	return stdout.Bytes(), nil
}

func (e *RawExec) pluginErr(err error, stdout, stderr []byte) error {
	emsg := types.Error{}
	if len(stdout) == 0 {
		if len(stderr) == 0 {
			/*标准输出及标准错误输出均无内容*/
			emsg.Msg = fmt.Sprintf("netplugin failed with no error message: %v", err)
		} else {
			/*标准输出无内容，但标准错误输出有内容*/
			emsg.Msg = fmt.Sprintf("netplugin failed: %q", string(stderr))
		}
	} else if perr := json.Unmarshal(stdout, &emsg); perr != nil {
		/*标准输出有内容，标准错误输出将被忽略，但在格式化输出时出错*/
		emsg.Msg = fmt.Sprintf("netplugin failed but error parsing its diagnostic message %q: %v", string(stdout), perr)
	}
	return &emsg
}

/*在paths列表中查找plugin,获得其绝对路径*/
func (e *RawExec) FindInPath(plugin string/*插件名称*/, paths []string/*可查询插件路径列表*/) (string, error) {
	return FindInPath(plugin, paths)
}
