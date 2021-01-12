package gen

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	. "github.com/dave/jennifer/jen"
)

var analyzedStructs []analyzedStruct

const (
	pkTop = "github.com/shamaton/msgpackgen/msgpack"
	pkEnc = pkTop + "/enc"
	pkDec = pkTop + "/dec"

	idEncoder = "encoder"
	idDecoder = "decoder"

	outputFileName = "resolver.msgpackgen.go"
)

// todo : tagをmapのcaseに使いつつ、変数に代入するようにしないといけない

var funcIdMap = map[string]string{}

type Generator struct {
	fileSet                *token.FileSet
	targetPackages         map[string]bool
	parseFiles             []*ast.File
	fileNames              []string
	file2FullPackageName   map[string]string
	file2PackageName       map[string]string
	fullpackage2ParseFiles map[string][]*ast.File
	noUserQualMap          map[string]bool

	outputDir           string
	outputPackageName   string
	outputPackagePrefix string

	pointer int
	verbose bool
	strict  bool
}

func (g *Generator) outputPackageFullName() string {
	return fmt.Sprintf("%s/%s", g.outputPackagePrefix, g.outputPackageName)
}

type analyzedStruct struct {
	PackageName string
	Name        string
	Fields      []analyzedField
	NoUseQual   bool
}

type analyzedField struct {
	Name string
	Tag  string
	Type types.Type
	Ast  *analyzedASTFieldType
}

func NewGenerator(pointer int, strict, verbose bool) *Generator {
	return &Generator{
		pointer:              pointer,
		strict:               strict,
		verbose:              verbose,
		targetPackages:       map[string]bool{},
		parseFiles:           []*ast.File{},
		fileNames:            []string{},
		file2FullPackageName: map[string]string{},
		file2PackageName:     map[string]string{},
		noUserQualMap:        map[string]bool{},
	}
}

func (g *Generator) Run(input, out string) error {

	outAbs, err := filepath.Abs(out)
	if err != nil {
		return err
	}

	g.outputDir = outAbs
	paths := strings.SplitN(g.outputDir, "src/", 2)
	if len(paths) != 2 {
		return fmt.Errorf("%s get import path failed", out)
	}
	g.outputPackageName = paths[1]

	// todo : ファイル指定オプション

	targets, err := g.getTargetFiles(input)
	if err != nil {
		return err
	}
	if len(targets) < 1 {
		return fmt.Errorf("not found go file")
	}

	err = g.getPackages(targets)
	if err != nil {
		return err
	}
	g.createAnalyzedStructs()
	analyzedStructs = g.filter(analyzedStructs)
	g.generateCode()
	return nil
}

func (g *Generator) getTargetFiles(dir string) ([]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, file := range files {
		if file.IsDir() {
			path, err := g.getTargetFiles(filepath.Join(dir, file.Name()))
			if err != nil {
				return nil, err
			}
			paths = append(paths, path...)
			continue
		}
		if filepath.Ext(file.Name()) == ".go" && !strings.HasSuffix(file.Name(), "_test.go") {
			paths = append(paths, filepath.Join(dir, file.Name()))
		}
	}

	var absPaths []string
	for _, path := range paths {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		absPaths = append(absPaths, abs)
	}
	return absPaths, nil
}
func privateFuncNamePattern(funcName string) string {
	return fmt.Sprintf("___%s", funcName)
}

func (g *Generator) filter(sts []analyzedStruct) []analyzedStruct {
	newStructs := make([]analyzedStruct, 0)
	allOk := true
	for _, v := range sts {
		ok := true
		var reasons []string
		for _, field := range v.Fields {
			if canGen, msgs := field.Ast.CanGenerate(sts); !canGen {
				ok = false
				reasons = append(reasons, msgs...)
			}
		}
		if !ok {
			fmt.Printf("can not generate %s.%s\n", v.PackageName, v.Name)
			fmt.Println("reason :", strings.Join(reasons, "\n"))
		} else {
			newStructs = append(newStructs, v)
		}
		allOk = allOk && ok
	}
	if !allOk {
		return g.filter(newStructs)
	} else {
		return newStructs
	}
}

func (g *Generator) generateCode() {

	for _, st := range analyzedStructs {
		funcIdMap[st.PackageName] = fmt.Sprintf("%x", sha256.Sum256([]byte(st.PackageName)))
	}

	// todo : ソースコードが存在している場所だったら、そちらにパッケージ名をあわせる
	f := NewFilePath(g.outputPackageFullName())

	registerName := "RegisterGeneratedResolver"
	f.HeaderComment("// Code generated by msgpackgen. DO NOT EDIT.\n// Thank you for using and generating.")
	f.Comment(fmt.Sprintf("// %s registers generated resolver.\n", registerName)).
		Func().Id(registerName).Params().Block(
		Qual(pkTop, "SetResolver").Call(
			Id(privateFuncNamePattern("encodeAsMap")),
			Id(privateFuncNamePattern("encodeAsArray")),
			Id(privateFuncNamePattern("decodeAsMap")),
			Id(privateFuncNamePattern("decodeAsArray")),
		),
	)

	g.decodeTopTemplate("decode", f).Block(
		If(Qual(pkTop, "StructAsArray").Call()).Block(
			Return(Id(privateFuncNamePattern("decodeAsArray")).Call(Id("data"), Id("i"))),
		).Else().Block(
			Return(Id(privateFuncNamePattern("decodeAsMap")).Call(Id("data"), Id("i"))),
		),
	)

	encReturn := Return(Nil(), Nil())
	decReturn := Return(False(), Nil())
	if g.strict {
		encReturn = Return(Nil(), Qual("fmt", "Errorf").Call(Lit("use strict option : undefined type")))
		decReturn = Return(False(), Qual("fmt", "Errorf").Call(Lit("use strict option : undefined type")))
	}

	g.decodeTopTemplate("decodeAsArray", f).Block(
		Switch(Id("v").Op(":=").Id("i").Assert(Type())).Block(
			g.decodeAsArrayCases()...,
		),
		decReturn,
	)

	g.decodeTopTemplate("decodeAsMap", f).Block(
		Switch(Id("v").Op(":=").Id("i").Assert(Type())).Block(
			g.decodeAsMapCases()...,
		),
		decReturn,
	)

	g.encodeTopTemplate("encode", f).Block(
		If(Qual(pkTop, "StructAsArray").Call()).Block(
			Return(Id(privateFuncNamePattern("encodeAsArray")).Call(Id("i"))),
		).Else().Block(
			Return(Id(privateFuncNamePattern("encodeAsMap")).Call(Id("i"))),
		),
	)

	g.encodeTopTemplate("encodeAsArray", f).Block(
		Switch(Id("v").Op(":=").Id("i").Assert(Type())).Block(
			g.encodeAsArrayCases()...,
		),
		encReturn,
	)

	g.encodeTopTemplate("encodeAsMap", f).Block(
		Switch(Id("v").Op(":=").Id("i").Assert(Type())).Block(
			g.encodeAsMapCases()...,
		),
		encReturn,
	)

	// todo : 名称修正
	for _, st := range analyzedStructs {
		st.calcFunction(f)
	}

	if err := os.MkdirAll(g.outputDir, 0777); err != nil {
		fmt.Println(err)
	}

	fileName := g.outputDir + "/" + outputFileName
	file, err := os.Create(fileName)
	if err != nil {
		fmt.Println(err)
	}
	defer file.Close()

	_, err = fmt.Fprintf(file, "%#v", f)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(fileName, "generated.")

}

func (g *Generator) decodeTopTemplate(name string, f *File) *Statement {
	return f.Comment(fmt.Sprintf("// %s\n", name)).
		Func().Id(privateFuncNamePattern(name)).Params(Id("data").Index().Byte(), Id("i").Interface()).Params(Bool(), Error())
}

func (g *Generator) encodeTopTemplate(name string, f *File) *Statement {
	return f.Comment(fmt.Sprintf("// %s\n", name)).
		Func().Id(privateFuncNamePattern(name)).Params(Id("i").Interface()).Params(Index().Byte(), Error())
}

func (g *Generator) encodeAsArrayCases() []Code {
	var states []Code
	for _, v := range analyzedStructs {

		var caseStatement func(string) *Statement
		var errID *Statement
		if v.NoUseQual {
			caseStatement = func(op string) *Statement { return Op(op).Id(v.Name) }
			errID = Lit(v.Name)
		} else {
			caseStatement = func(op string) *Statement { return Op(op).Qual(v.PackageName, v.Name) }
			errID = Lit(v.PackageName + "." + v.Name)
		}

		f := func(ptr string) *Statement {
			return Case(caseStatement(ptr)).Block(
				Id(idEncoder).Op(":=").Qual(pkEnc, "NewEncoder").Call(),
				List(Id("size"), Err()).Op(":=").Id(v.calcArraySizeFuncName()).Call(Id(ptr+"v"), Id(idEncoder)),
				If(Err().Op("!=").Nil()).Block(
					Return(Nil(), Err()),
				),
				Id(idEncoder).Dot("MakeBytes").Call(Id("size")),
				List(Id("b"), Id("offset"), Err()).Op(":=").Id(v.encodeArrayFuncName()).Call(Id(ptr+"v"), Id(idEncoder), Lit(0)),
				If(Err().Op("!=").Nil()).Block(
					Return(Nil(), Err()),
				),
				If(Id("size").Op("!=").Id("offset")).Block(
					Return(Nil(), Qual("fmt", "Errorf").Call(Lit("%s size / offset different %d : %d"), errID, Id("size"), Id("offset"))),
				),
				Return(Id("b"), Err()),
			)
		}

		states = append(states, f(""))

		if g.pointer > 0 {
			states = append(states, f("*"))
		}

		for i := 0; i < g.pointer-1; i++ {
			ptr := strings.Repeat("*", i+2)
			states = append(states, Case(caseStatement(ptr)).Block(
				Return(Id(privateFuncNamePattern("encodeAsArray")).Call(Id("*v"))),
			))
		}
	}
	return states
}

func (g *Generator) encodeAsMapCases() []Code {
	var states []Code
	for _, v := range analyzedStructs {

		var caseStatement func(string) *Statement
		var errID *Statement
		if v.NoUseQual {
			caseStatement = func(op string) *Statement { return Op(op).Id(v.Name) }
			errID = Lit(v.Name)
		} else {
			caseStatement = func(op string) *Statement { return Op(op).Qual(v.PackageName, v.Name) }
			errID = Lit(v.PackageName + "." + v.Name)
		}

		f := func(ptr string) *Statement {
			return Case(caseStatement(ptr)).Block(
				Id(idEncoder).Op(":=").Qual(pkEnc, "NewEncoder").Call(),
				List(Id("size"), Err()).Op(":=").Id(v.calcMapSizeFuncName()).Call(Id(ptr+"v"), Id(idEncoder)),
				If(Err().Op("!=").Nil()).Block(
					Return(Nil(), Err()),
				),
				Id(idEncoder).Dot("MakeBytes").Call(Id("size")),
				List(Id("b"), Id("offset"), Err()).Op(":=").Id(v.encodeMapFuncName()).Call(Id(ptr+"v"), Id(idEncoder), Lit(0)),
				If(Err().Op("!=").Nil()).Block(
					Return(Nil(), Err()),
				),
				If(Id("size").Op("!=").Id("offset")).Block(
					Return(Nil(), Qual("fmt", "Errorf").Call(Lit("%s size / offset different %d : %d"), errID, Id("size"), Id("offset"))),
				),
				Return(Id("b"), Err()),
			)
		}

		states = append(states, f(""))

		if g.pointer > 0 {
			states = append(states, f("*"))
		}

		for i := 0; i < g.pointer-1; i++ {
			ptr := strings.Repeat("*", i+2)
			states = append(states, Case(caseStatement(ptr)).Block(
				Return(Id(privateFuncNamePattern("encodeAsMap")).Call(Id("*v"))),
			))
		}
	}
	return states
}

func (g *Generator) decodeAsArrayCases() []Code {
	var states []Code
	for _, v := range analyzedStructs {

		var caseStatement func(string) *Statement
		if v.NoUseQual {
			caseStatement = func(op string) *Statement { return Op(op).Id(v.Name) }
		} else {
			caseStatement = func(op string) *Statement { return Op(op).Qual(v.PackageName, v.Name) }
		}

		states = append(states, Case(caseStatement("*")).Block(
			List(Id("_"), Err()).Op(":=").Id(v.decodeArrayFuncName()).Call(Id("v"), Qual(pkDec, "NewDecoder").Call(Id("data")), Id("0")),
			Return(True(), Err())))

		if g.pointer > 0 {
			states = append(states, Case(caseStatement("**")).Block(
				List(Id("_"), Err()).Op(":=").Id(v.decodeArrayFuncName()).Call(Id("*v"), Qual(pkDec, "NewDecoder").Call(Id("data")), Id("0")),
				Return(True(), Err())))
		}

		for i := 0; i < g.pointer-1; i++ {
			ptr := strings.Repeat("*", i+3)
			states = append(states, Case(caseStatement(ptr)).Block(
				Return(Id(privateFuncNamePattern("decodeAsArray")).Call(Id("data"), Id("*v"))),
			))
		}
	}
	return states
}

func (g *Generator) decodeAsMapCases() []Code {
	var states []Code
	for _, v := range analyzedStructs {

		var caseStatement func(string) *Statement
		if v.NoUseQual {
			caseStatement = func(op string) *Statement { return Op(op).Id(v.Name) }
		} else {
			caseStatement = func(op string) *Statement { return Op(op).Qual(v.PackageName, v.Name) }
		}

		states = append(states, Case(caseStatement("*")).Block(
			List(Id("_"), Err()).Op(":=").Id(v.decodeMapFuncName()).Call(Id("v"), Qual(pkDec, "NewDecoder").Call(Id("data")), Id("0")),
			Return(True(), Err())))

		if g.pointer > 0 {
			states = append(states, Case(caseStatement("**")).Block(
				List(Id("_"), Err()).Op(":=").Id(v.decodeMapFuncName()).Call(Id("*v"), Qual(pkDec, "NewDecoder").Call(Id("data")), Id("0")),
				Return(True(), Err())))
		}

		for i := 0; i < g.pointer-1; i++ {
			ptr := strings.Repeat("*", i+3)
			states = append(states, Case(caseStatement(ptr)).Block(
				Return(Id(privateFuncNamePattern("decodeAsArray")).Call(Id("data"), Id("*v"))),
			))
		}
	}
	return states
}

func (as *analyzedStruct) calcArraySizeFuncName() string {
	return createFuncName("calcArraySize", as.Name, as.PackageName)
}

func (as *analyzedStruct) calcMapSizeFuncName() string {
	return createFuncName("calcMapSize", as.Name, as.PackageName)
}

func (as *analyzedStruct) encodeArrayFuncName() string {
	return createFuncName("encodeArray", as.Name, as.PackageName)
}

func (as *analyzedStruct) encodeMapFuncName() string {
	return createFuncName("encodeMap", as.Name, as.PackageName)
}

func (as *analyzedStruct) decodeArrayFuncName() string {
	return createFuncName("decodeArray", as.Name, as.PackageName)
}

func (as *analyzedStruct) decodeMapFuncName() string {
	return createFuncName("decodeMap", as.Name, as.PackageName)
}

func createFuncName(prefix, name, packageName string) string {
	return privateFuncNamePattern(fmt.Sprintf("%s%s_%s", prefix, name, funcIdMap[packageName]))
}
