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

package convert

import (
	"fmt"

	"github.com/containernetworking/cni/pkg/types"
)

type ResultFactoryFunc func([]byte) (types.Result, error)

type creator struct {
	// CNI Result spec versions that createFn can create a Result for
	versions []string /*支持的一组版本*/
	/*此版本对应的createFn*/
	createFn ResultFactoryFunc
}

var creators []*creator

/*查找version对应的creator*/
func findCreator(version string) *creator {
	for _, c := range creators {
		for _, v := range c.versions {
			if v == version {
				return c
			}
		}
	}
	return nil
}

// Create creates a CNI Result using the given JSON, or an error if the creation
// could not be performed
func Create(version string/*要解析为的版本号*/, bytes []byte/*要解析的内容*/) (types.Result, error) {
	if c := findCreator(version); c != nil {
		/*通过此createFn解析此字符串*/
		return c.createFn(bytes)
	}
	return nil, fmt.Errorf("unsupported CNI result version %q", version)
}

// RegisterCreator registers a CNI Result creator. SHOULD NOT BE CALLED
// EXCEPT FROM CNI ITSELF.
func RegisterCreator(versions []string, createFn ResultFactoryFunc) {
	// Make sure there is no creator already registered for these versions
	for _, v := range versions {
		if findCreator(v) != nil {
			/*此version对应的creator已注册*/
			panic(fmt.Sprintf("creator already registered for %s", v))
		}
	}
	creators = append(creators, &creator{
		versions: versions,/*版本号*/
		createFn: createFn,/*对应的createFn*/
	})
}
