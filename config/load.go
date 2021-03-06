// Copyright 2013 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// NOTE: This parser is non-reentrant due to its dependence on global state.

// GoLex sadly needs these global variables for storing temporary token/parsing information.
var yylval *yySymType    // For storing extra token information, like the contents of a string.
var yyline int           // Line number within the current file or buffer.
var yypos int            // Character position within the current line.
var parsedConfig *Config // Temporary variable for storing the parsed configuration.

type ConfigLexer struct {
	errors []string
}

func (lexer *ConfigLexer) Lex(lval *yySymType) int {
	yylval = lval
	token_type := yylex()
	return token_type
}

func (lexer *ConfigLexer) Error(errorStr string) {
	err := fmt.Sprintf("Error reading config at line %v, char %v: %v", yyline, yypos, errorStr)
	lexer.errors = append(lexer.errors, err)
}

func LoadFromReader(configReader io.Reader) (*Config, error) {
	parsedConfig = New()
	yyin = configReader
	yypos = 1
	yyline = 1
	yydata = ""
	yytext = ""

	lexer := &ConfigLexer{}
	yyParse(lexer)

	if len(lexer.errors) > 0 {
		err := errors.New(strings.Join(lexer.errors, "\n"))
		return &Config{}, err
	}

	return parsedConfig, nil
}

func LoadFromString(configString string) (*Config, error) {
	configReader := strings.NewReader(configString)
	return LoadFromReader(configReader)
}

func LoadFromFile(fileName string) (*Config, error) {
	configReader, err := os.Open(fileName)
	if err != nil {
		return &Config{}, err
	}
	defer configReader.Close()

	return LoadFromReader(configReader)
}
