// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package config

import (
	"fmt"
	"log"
	"os"

	uconfig "go.uber.org/config"
	"gopkg.in/validator.v2"
)

const (
	// EnvKeyRoot the environment variable key for runtime root dir
	EnvKeyRoot = "CADENCE_ROOT"
	// EnvKeyConfigDir the environment variable key for config dir
	EnvKeyConfigDir = "CADENCE_CONFIG_DIR"
	// EnvKeyEnvironment is the environment variable key for environment
	EnvKeyEnvironment = "CADENCE_ENVIRONMENT"
	// EnvKeyAvailabilityZone is the environment variable key for AZ
	EnvKeyAvailabilityZone = "CADENCE_AVAILABILTY_ZONE"
)

const (
	baseFile         = "base.yaml"
	envDevelopment   = "development"
	defaultConfigDir = "config"
)

// Load loads the configuration from a set of
// yaml config files found in the config directory
//
// The loader first fetches the set of files matching
// a pre-determined naming convention, then sorts
// them by hierarchy order and after that, simply
// loads the files one after another with the
// key/values in the later files overriding the key/values
// in the earlier files
//
// The hierarchy is as follows from lowest to highest
//
//	base.yaml
//	    env.yaml   -- environment is one of the input params ex-development
//	      env_az.yaml -- zone is another input param
func Load(env string, configDir string, zone string, config interface{}) error {

	if len(env) == 0 {
		env = envDevelopment
	}

	if len(configDir) == 0 {
		configDir = defaultConfigDir
	}

	files, err := getConfigFiles(env, configDir, zone)
	if err != nil {
		return fmt.Errorf("unable to get config files: %w", err)
	}

	log.Printf("Loading configFiles=%v\n", files)

	var options []uconfig.YAMLOption
	for _, f := range files {
		options = append(options, uconfig.File(f))
	}

	// expand env variables declared in .yaml files
	options = append(options, uconfig.Expand(os.LookupEnv))

	yaml, err := uconfig.NewYAML(options...)
	if err != nil {
		return fmt.Errorf("unable to create yaml parser: %w", err)
	}

	err = yaml.Get(uconfig.Root).Populate(config)
	if err != nil {
		return fmt.Errorf("unable to populate config: %w", err)
	}

	err = validator.Validate(config)
	if err != nil {
		return fmt.Errorf("failed to validate config: %w", err)
	}
	return nil
}

// getConfigFiles returns the list of config files to
// process in the hierarchy order
func getConfigFiles(env string, configDir string, zone string) ([]string, error) {

	candidates := []string{
		path(configDir, baseFile),
		path(configDir, file(env, "yaml")),
	}

	if len(zone) > 0 {
		f := file(concat(env, zone), "yaml")
		candidates = append(candidates, path(configDir, f))
	}

	var result []string

	for _, c := range candidates {
		if _, err := os.Stat(c); err != nil {
			continue
		}
		result = append(result, c)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no config files found within %v", configDir)
	}

	return result, nil
}

func concat(a, b string) string {
	return a + "_" + b
}

func file(name string, suffix string) string {
	return name + "." + suffix
}

func path(dir string, file string) string {
	return dir + "/" + file
}
