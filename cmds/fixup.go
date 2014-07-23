package cmds

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"go/ast"
	"go/parser"
	"go/token"

	"github.com/maximilien/i18n4go/common"
)

type Fixup struct {
	options common.Options

	I18nStringInfos []common.I18nStringInfo
	English         []common.I18nStringInfo
	Source          map[string]int
	Locales         map[string]map[string]string
}

func NewFixup(options common.Options) Fixup {
	return Fixup{
		options:         options,
		I18nStringInfos: []common.I18nStringInfo{},
	}
}

func (fu *Fixup) Options() common.Options {
	return fu.options
}

func (fu *Fixup) Println(a ...interface{}) (int, error) {
	if fu.options.VerboseFlag {
		return fmt.Println(a...)
	}

	return 0, nil
}

func (fu *Fixup) Printf(msg string, a ...interface{}) (int, error) {
	if fu.options.VerboseFlag {
		return fmt.Printf(msg, a...)
	}

	return 0, nil
}

func (fu *Fixup) Run() error {
	//FIND PROBLEMS HERE AND RETURN AN ERROR
	source, err := fu.findSourceStrings()
	fu.Source = source

	if err != nil {
		fmt.Println(fmt.Sprintf("Couldn't find any source strings: %s", err.Error()))
		return err
	}

	locales := findTranslationFiles(".")

	englishFile := locales["en_US"][0]
	if englishFile == "" {
		fmt.Println("Could not find an i18n file for locale: en_US")
		return errors.New("Could not find an i18n file for locale: en_US")
	}

	englishStringInfos, err := fu.findI18nStrings(englishFile)

	if err != nil {
		fmt.Println(fmt.Sprintf("Couldn't find the english strings: %s", err.Error()))
		return err
	}

	additionalTranslations := getAdditionalTranslations(source, englishStringInfos)
	removedTranslations := getRemovedTranslations(source, englishStringInfos)

	updatedTranslations := map[string]string{}

	if len(additionalTranslations) > 0 {
		for index, newUpdatedTranslation := range additionalTranslations {
			if len(removedTranslations) > 0 {
				var input string

				escape := true

				for escape {
					fmt.Printf("Is the string \"%s\" a new or updated string? [new/upd]\n", newUpdatedTranslation)

					_, err := fmt.Scanf("%s\n", &input)

					if err != nil {
						panic(err)
					}

					input = strings.ToLower(input)

					switch input {
					case "new":
						/*for i, key := range diffSlaveToMaster {
							fmt.Printf("%d: %s\n", i+1, key)
						}

						var number int
						fmt.Scanf("%d\n", &number)
						// Check input
						if number <= 0 || number > len(diffSlaveToMaster) {
							goto chooseKey
						}

						// Get old ID, add the new ID to english, delete old ID.
						updateString := diffSlaveToMaster[number-1]
						fu.English[id] = id
						delete(fu.English, updateString)
						masterUpdates[updateString] = id

						diffSlaveToMaster = removeFromSlice(diffSlaveToMaster, number-1)
						// Take previous key and change to new key in all translations
						// (change translation in english also), mark as dirty.
						*/
						escape = false
					case "upd":
						fmt.Println("Select the number for the previous translation:")
						for index, value := range removedTranslations {
							fmt.Printf("\t%d. %s\n", (index + 1), value)
						}

						var updSelection int
						_, err := fmt.Scanf("%d\n", &updSelection)

						if err == nil && updSelection > 0 && updSelection <= len(removedTranslations) {
							updSelection = updSelection - 1

							updatedTranslations[removedTranslations[updSelection]] = newUpdatedTranslation

							removedTranslations = removeFromSlice(removedTranslations, updSelection)
							additionalTranslations = removeFromSlice(additionalTranslations, index)
						}
						escape = false
					case "exit":
						fmt.Println("Canceling fixup")
						os.Exit(0)
					default:
						fmt.Println("Invalid response.")
					}
				}
			}
		}
	}

	fmt.Println("UPDATES: ", updatedTranslations)

	for locale, i18nFiles := range locales {
		translatedStrings, err := fu.findI18nStrings(i18nFiles[0])
		if err != nil {
			fmt.Println(fmt.Sprintf("Couldn't get the strings from %s: %s", locale, err.Error()))
			return err
		}

		if len(updatedTranslations) > 0 {
			updateTranslations(translatedStrings, i18nFiles[0], updatedTranslations)
		}

		if len(additionalTranslations) > 0 {
			addTranslations(translatedStrings, i18nFiles[0], additionalTranslations)
		}

		if len(removedTranslations) > 0 {
			removeTranslations(translatedStrings, i18nFiles[0], removedTranslations)
		}

		err = writeStringInfoMapToJSON(translatedStrings, i18nFiles[0])
	}

	if err == nil {
		fmt.Printf("OK")
	}

	return err
}

func (fu *Fixup) inspectFile(file string) (translatedStrings []string, err error) {
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, file, nil, parser.AllErrors)
	if err != nil {
		fu.Println(err)
		return
	}

	ast.Inspect(astFile, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			switch x.Fun.(type) {
			case *ast.Ident:
				funName := x.Fun.(*ast.Ident).Name

				if funName == "T" || funName == "t" {
					if stringArg, ok := x.Args[0].(*ast.BasicLit); ok {
						translatedString, err := strconv.Unquote(stringArg.Value)
						if err != nil {
							panic(err.Error())
						}
						translatedStrings = append(translatedStrings, translatedString)
					}
				}
			default:
				//Skip!
			}
		}
		return true
	})

	return
}

func (fu *Fixup) findSourceStrings() (sourceStrings map[string]int, err error) {
	sourceStrings = make(map[string]int)
	files := getGoFiles(".")

	for _, file := range files {
		fileStrings, err := fu.inspectFile(file)
		if err != nil {
			fmt.Println("Error when inspecting go file: ", file)
			return sourceStrings, err
		}

		for _, string := range fileStrings {
			sourceStrings[string]++
		}
	}

	return
}

func (fu *Fixup) findI18nStrings(i18nFile string) (i18nStrings map[string]common.I18nStringInfo, err error) {
	i18nStrings = make(map[string]common.I18nStringInfo)

	stringInfos, err := common.LoadI18nStringInfos(i18nFile)

	if err != nil {
		return nil, err
	}

	return common.CreateI18nStringInfoMap(stringInfos)
}

func getAdditionalTranslations(sourceTranslations map[string]int, englishTranslations map[string]common.I18nStringInfo) []string {
	additionalTranslations := []string{}

	for id, _ := range sourceTranslations {
		if _, ok := englishTranslations[id]; !ok {
			additionalTranslations = append(additionalTranslations, id)
		}
	}
	return additionalTranslations
}

func getRemovedTranslations(sourceTranslations map[string]int, englishTranslations map[string]common.I18nStringInfo) []string {
	removedTranslations := []string{}

	for id, _ := range englishTranslations {
		if _, ok := sourceTranslations[id]; !ok {
			removedTranslations = append(removedTranslations, id)
		}
	}

	return removedTranslations
}

func writeStringInfoMapToJSON(localeMap map[string]common.I18nStringInfo, localeFile string) error {
	localeArray := common.I18nStringInfoMapValues2Array(localeMap)
	encodedLocale, err := json.MarshalIndent(localeArray, "", "   ")
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(localeFile, encodedLocale, 0644)
	if err != nil {
		return err
	}
	return nil
}

func addTranslations(localeMap map[string]common.I18nStringInfo, localeFile string, addTranslations []string) {
	fmt.Printf("Adding these strings to the %s translation file:\n", localeFile)

	for _, id := range addTranslations {
		localeMap[id] = common.I18nStringInfo{ID: id, Translation: id}
		fmt.Println(id)
	}
}

func removeTranslations(localeMap map[string]common.I18nStringInfo, localeFile string, remTranslations []string) error {
	var err error
	fmt.Printf("Removing these strings from the %s translation file:\n", localeFile)

	for _, id := range remTranslations {
		delete(localeMap, id)
		fmt.Println(id)
	}

	return err
}

func updateTranslations(localMap map[string]common.I18nStringInfo, localeFile string, updTranslations map[string]string) {
	fmt.Printf("Updating these strings from the %s translation file:\n", localeFile)

	for key, value := range updTranslations {
		if localeFile == "en_US" {
			localMap[value] = common.I18nStringInfo{ID: value, Translation: value}
		} else {
			localMap[value] = common.I18nStringInfo{ID: value, Translation: localMap[key].Translation, Dirty: true}
		}
		delete(localMap, key)
	}
}

func removeFromSlice(slice []string, index int) []string {
	return append(slice[:index], slice[index+1:]...)
}