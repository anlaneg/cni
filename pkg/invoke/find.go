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

package invoke

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FindInPath returns the full path of the plugin by searching in the provided path
func FindInPath(plugin string/*插件名称*/, paths []string/*查询插件的路径*/) (string, error) {
	if plugin == "" {
		/*plugin名称不能为空*/
		return "", fmt.Errorf("no plugin name provided")
	}

	if strings.ContainsRune(plugin, os.PathSeparator) {
		/*plgin名称中不得有路径分隔符*/
		return "", fmt.Errorf("invalid plugin name: %s", plugin)
	}

	if len(paths) == 0 {
		/*待查询路径的数组不得为空*/
		return "", fmt.Errorf("no paths provided")
	}

	/*在paths数组中查找plugin，考虑可执行后缀，如果文件存在，返回全路径*/
	for _, path := range paths {
		for _, fe := range ExecutableFileExtensions/*可执行文件后缀*/ {
			fullpath := filepath.Join(path, plugin) + fe
			if fi, err := os.Stat(fullpath); err == nil && fi.Mode().IsRegular() {
				/*此文件存在，则返回文件全路径（返回的为首个发现的）*/
				return fullpath, nil
			}
		}
	}

	return "", fmt.Errorf("failed to find plugin %q in path %s", plugin, paths)
}
