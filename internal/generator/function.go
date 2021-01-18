package generator

import (
	"fmt"
	"math"
	"strings"

	. "github.com/dave/jennifer/jen"
)

func (as *analyzedStruct) calcFunction(f *File) {
	v := "v"

	calcStruct, encStructArray, encStructMap := as.CreateStructCode(len(as.Fields))

	calcArraySizeCodes := make([]Code, 0)
	calcArraySizeCodes = append(calcArraySizeCodes, Id("size").Op(":=").Lit(0))
	calcArraySizeCodes = append(calcArraySizeCodes, calcStruct)

	calcMapSizeCodes := make([]Code, 0)
	calcMapSizeCodes = append(calcMapSizeCodes, Id("size").Op(":=").Lit(0))
	calcMapSizeCodes = append(calcMapSizeCodes, calcStruct)

	encArrayCodes := make([]Code, 0)
	encArrayCodes = append(encArrayCodes, Var().Err().Error())
	encArrayCodes = append(encArrayCodes, encStructArray)

	encMapCodes := make([]Code, 0)
	encMapCodes = append(encMapCodes, Var().Err().Error())
	encMapCodes = append(encMapCodes, encStructMap)

	decArrayCodes := make([]Code, 0)
	decArrayCodes = append(decArrayCodes, List(Id("offset"), Err()).Op(":=").Id(idDecoder).Dot("CheckStructHeader").Call(Lit(len(as.Fields)), Id("offset")))
	decArrayCodes = append(decArrayCodes, If(Err().Op("!=").Nil()).Block(
		Return(Lit(0), Err()),
	))

	decMapCodeSwitchCases := make([]Code, 0)

	for _, field := range as.Fields {
		fieldName := "v." + field.Name

		calcKeyStringCode, writeKeyStringCode := as.CreateKeyStringCode(field.Tag)
		calcMapSizeCodes = append(calcMapSizeCodes, calcKeyStringCode)
		encMapCodes = append(encMapCodes, writeKeyStringCode)

		cArray, cMap, eArray, eMap, dArray, dMap, _ := as.createFieldCode(field.Ast, fieldName, fieldName)
		calcArraySizeCodes = append(calcArraySizeCodes, cArray...)

		calcMapSizeCodes = append(calcMapSizeCodes, cMap...)

		encArrayCodes = append(encArrayCodes, eArray...)
		encMapCodes = append(encMapCodes, eMap...)

		decArrayCodes = append(decArrayCodes, dArray...)

		decMapCodeSwitchCases = append(decMapCodeSwitchCases, Case(Lit(field.Tag)).Block(dMap...))

	}

	decMapCodeSwitchCases = append(decMapCodeSwitchCases, Default().Block(Id("offset").Op("=").Id(idDecoder).Dot("JumpOffset").Call(Id("offset"))))

	decMapCodes := make([]Code, 0)
	decMapCodes = append(decMapCodes, List(Id("offset"), Err()).Op(":=").Id(idDecoder).Dot("CheckStructHeader").Call(Lit(len(as.Fields)), Id("offset")))
	decMapCodes = append(decMapCodes, If(Err().Op("!=").Nil()).Block(
		Return(Lit(0), Err()),
	))
	decMapCodes = append(decMapCodes, Id("dataLen").Op(":=").Id(idDecoder).Dot("Len").Call())
	decMapCodes = append(decMapCodes, For(Id("offset").Op("<").Id("dataLen").Block(
		Var().Id("s").String(),
		List(Id("s"), Id("offset"), Err()).Op("=").Id(idDecoder).Dot("AsString").Call(Id("offset")),
		If(Err().Op("!=").Nil()).Block(
			Return(Lit(0), Err()),
		),
		Switch(Id("s")).Block(
			decMapCodeSwitchCases...,
		),
	)))

	var firstEncParam, firstDecParam *Statement
	if as.NoUseQual {
		firstEncParam = Id(v).Id(as.Name)
		firstDecParam = Id(v).Op("*").Id(as.Name)
	} else {
		firstEncParam = Id(v).Qual(as.PackageName, as.Name)
		firstDecParam = Id(v).Op("*").Qual(as.PackageName, as.Name)
	}

	f.Comment(fmt.Sprintf("// calculate size from %s.%s\n", as.PackageName, as.Name)).
		Func().Id(as.calcArraySizeFuncName()).Params(firstEncParam, Id(idEncoder).Op("*").Qual(pkEnc, "Encoder")).Params(Int(), Error()).Block(
		append(calcArraySizeCodes, Return(Id("size"), Nil()))...,
	)

	f.Comment(fmt.Sprintf("// calculate size from %s.%s\n", as.PackageName, as.Name)).
		Func().Id(as.calcMapSizeFuncName()).Params(firstEncParam, Id(idEncoder).Op("*").Qual(pkEnc, "Encoder")).Params(Int(), Error()).Block(
		append(calcMapSizeCodes, Return(Id("size"), Nil()))...,
	)

	f.Comment(fmt.Sprintf("// encode from %s.%s\n", as.PackageName, as.Name)).
		Func().Id(as.encodeArrayFuncName()).Params(firstEncParam, Id(idEncoder).Op("*").Qual(pkEnc, "Encoder"), Id("offset").Int()).Params(Index().Byte(), Int(), Error()).Block(
		append(encArrayCodes, Return(Id(idEncoder).Dot("EncodedBytes").Call(), Id("offset"), Err()))...,
	)

	f.Comment(fmt.Sprintf("// encode from %s.%s\n", as.PackageName, as.Name)).
		Func().Id(as.encodeMapFuncName()).Params(firstEncParam, Id(idEncoder).Op("*").Qual(pkEnc, "Encoder"), Id("offset").Int()).Params(Index().Byte(), Int(), Error()).Block(
		append(encMapCodes, Return(Id(idEncoder).Dot("EncodedBytes").Call(), Id("offset"), Err()))...,
	)

	f.Comment(fmt.Sprintf("// decode to %s.%s\n", as.PackageName, as.Name)).
		Func().Id(as.decodeArrayFuncName()).Params(firstDecParam, Id(idDecoder).Op("*").Qual(pkDec, "Decoder"), Id("offset").Int()).Params(Int(), Error()).Block(
		append(decArrayCodes, Return(Id("offset"), Err()))...,
	)

	f.Comment(fmt.Sprintf("// decode to %s.%s\n", as.PackageName, as.Name)).
		Func().Id(as.decodeMapFuncName()).Params(firstDecParam, Id(idDecoder).Op("*").Qual(pkDec, "Decoder"), Id("offset").Int()).Params(Int(), Error()).Block(

		append(decMapCodes, Return(Id("offset"), Err()))...,
	)
}

func (as *analyzedStruct) CreateKeyStringCode(v string) (Code, Code) {
	l := len(v)
	suffix := ""
	if l < 32 {
		suffix = "Fix"
	} else if l <= math.MaxUint8 {
		suffix = "8"
	} else if l <= math.MaxUint16 {
		suffix = "16"
	} else {
		suffix = "32"
	}

	return Id("size").Op("+=").Id(idEncoder).Dot("CalcString" + suffix).Call(Lit(l)),
		Id("offset").Op("=").Id(idEncoder).Dot("WriteString"+suffix).Call(Lit(v), Lit(l), Id("offset"))
}

func (as *analyzedStruct) CreateStructCode(fieldNum int) (Code, Code, Code) {

	suffix := ""
	if fieldNum <= 0x0f {
		suffix = "Fix"
	} else if fieldNum <= math.MaxUint16 {
		suffix = "16"
	} else if uint(fieldNum) <= math.MaxUint32 {
		suffix = "32"
	}

	return Id("size").Op("+=").Id(idEncoder).Dot("CalcStructHeader" + suffix).Call(Lit(fieldNum)),
		Id("offset").Op("=").Id(idEncoder).Dot(" WriteStructHeader"+suffix+"AsArray").Call(Lit(fieldNum), Id("offset")),
		Id("offset").Op("=").Id(idEncoder).Dot(" WriteStructHeader"+suffix+"AsMap").Call(Lit(fieldNum), Id("offset"))
}

func (as *analyzedStruct) createFieldCode(ast *analyzedASTFieldType, encodeFieldName, decodeFieldName string) (cArray []Code, cMap []Code, eArray []Code, eMap []Code, dArray []Code, dMap []Code, err error) {

	switch {
	case ast.IsIdentical():
		return as.createBasicCode(ast, encodeFieldName, decodeFieldName)

	case ast.IsSlice():
		return as.createSliceCode(ast, encodeFieldName, decodeFieldName)

	case ast.IsArray():
		return as.createArrayCode(ast, encodeFieldName, decodeFieldName)

	case ast.IsMap():
		return as.createMapCode(ast, encodeFieldName, decodeFieldName)

	case ast.IsPointer():
		return as.createPointerCode(ast, encodeFieldName, decodeFieldName)

	case ast.IsStruct():

		ptrOp := ""
		node := ast
		for {
			if node.HasParent() && node.Parent.IsPointer() {
				ptrOp += "*"
				node = node.Parent
			} else {
				break
			}
		}

		fieldValue := Op(ptrOp).Id(encodeFieldName)

		// todo : ポインタでの動作検証
		if ast.ImportPath == "time" {
			cArray = append(cArray, as.addSizePattern1("CalcTime", fieldValue))
			eArray = append(eArray, as.encPattern1("WriteTime", fieldValue, Id("offset")))

			cMap = append(cMap, as.addSizePattern1("CalcTime", fieldValue))
			eMap = append(eMap, as.encPattern1("WriteTime", fieldValue, Id("offset")))

			dArray = append(dArray, as.decodeBasicPattern(ast, decodeFieldName, "offset", "AsDateTime")...)
			dMap = append(dMap, as.decodeBasicPattern(ast, decodeFieldName, "offset", "AsDateTime")...)
		} else {
			// todo : 対象のパッケージかどうかをちゃんと判断する
			cArray, cMap, eArray, eMap, dArray, dMap = as.createNamedCode(encodeFieldName, decodeFieldName, ast)
		}

	default:
		// todo : error

	}

	return cArray, cMap, eArray, eMap, dArray, dMap, err
}

func (as *analyzedStruct) createPointerCode(ast *analyzedASTFieldType, encodeFieldName, decodeFieldName string) (cArray []Code, cMap []Code, eArray []Code, eMap []Code, dArray []Code, dMap []Code, err error) {

	encodeChildName := encodeFieldName + "p"
	if isRootField(encodeFieldName) {
		encodeChildName = "vp"
	}

	ca, _, ea, _, da, _, _ := as.createFieldCode(ast.Elm(), encodeChildName, decodeFieldName)

	cArray = make([]Code, 0)
	cArray = append(cArray, If(Id(encodeFieldName).Op("!=").Nil()).Block(
		append([]Code{
			Id(encodeChildName).Op(":=").Op("*").Id(encodeFieldName),
		}, ca...)...,
	).Else().Block(
		Id("size").Op("+=").Id(idEncoder).Dot("CalcNil").Call(),
	))

	eArray = make([]Code, 0)
	eArray = append(eArray, If(Id(encodeFieldName).Op("!=").Nil()).Block(
		append([]Code{
			Id(encodeChildName).Op(":=").Op("*").Id(encodeFieldName),
		}, ea...)...,
	).Else().Block(
		Id("offset").Op("=").Id(idEncoder).Dot("WriteNil").Call(Id("offset")),
	))

	// todo : ようかくにん、重複コードをスキップ
	isParentPointer := ast.HasParent() && ast.Parent.IsPointer()
	if isParentPointer {
		dArray = da
	} else {
		dArray = make([]Code, 0)
		dArray = append(dArray, If(Op("!").Id(idDecoder).Dot("IsCodeNil").Call(Id("offset"))).Block(
			da...,
		).Else().Block(
			Id("offset").Op("++"),
		))
	}

	return cArray, cArray, eArray, eArray, dArray, dArray, err
}

func (as *analyzedStruct) createMapCode(ast *analyzedASTFieldType, encodeFieldName, decodeFieldName string) (cArray []Code, cMap []Code, eArray []Code, eMap []Code, dArray []Code, dMap []Code, err error) {

	key, value := ast.KeyValue()

	encodeChildKey, encodeChildValue := encodeFieldName+"k", encodeFieldName+"v"
	if isRootField(encodeFieldName) {
		encodeChildKey = "kk"
		encodeChildValue = "vv"
	}

	decodeChildKey, decodeChildValue := decodeFieldName+"k", decodeFieldName+"v"
	if isRootField(decodeFieldName) {
		decodeChildKey = "kk"
		decodeChildValue = "vv"
	}

	//ptrOp := ""
	andOp := ""
	node := ast
	for {
		if node.HasParent() && node.Parent.IsPointer() {
			//ptrOp += "*"
			andOp += "&"
			node = node.Parent
		} else {
			break
		}
	}

	caKey, _, eaKey, _, daKey, _, _ := as.createFieldCode(key, encodeChildKey, decodeChildKey)
	caValue, _, eaValue, _, daValue, _, _ := as.createFieldCode(value, encodeChildValue, decodeChildValue)

	calcCodes := as.addSizePattern2("CalcMapLength", Len(Id(encodeFieldName)))
	calcCodes = append(calcCodes, For(List(Id(encodeChildKey), Id(encodeChildValue)).Op(":=").Range().Id(encodeFieldName)).Block(
		append(caKey, caValue...)...,
	))

	cArray = append(cArray, If(Id(encodeFieldName).Op("!=").Nil()).Block(
		calcCodes...,
	).Else().Block(
		as.addSizePattern1("CalcNil"),
	))

	encCodes := make([]Code, 0)
	encCodes = append(encCodes, Id("offset").Op("=").Id(idEncoder).Dot("WriteMapLength").Call(Len(Id(encodeFieldName)), Id("offset")))
	encCodes = append(encCodes, For(List(Id(encodeChildKey), Id(encodeChildValue)).Op(":=").Range().Id(encodeFieldName)).Block(
		append(eaKey, eaValue...)...,
	))

	eArray = append(eArray, If(Id(encodeFieldName).Op("!=").Nil()).Block(
		encCodes...,
	).Else().Block(
		Id("offset").Op("=").Id(idEncoder).Dot("WriteNil").Call(Id("offset")),
	))

	decCodes := make([]Code, 0)
	decCodes = append(decCodes, ast.TypeJenChain(Var().Id(decodeChildValue)))
	decCodes = append(decCodes, Var().Id(decodeChildValue+"l").Int())
	decCodes = append(decCodes, List(Id(decodeChildValue+"l"), Id("offset"), Err()).Op("=").Id(idDecoder).Dot("MapLength").Call(Id("offset")))
	decCodes = append(decCodes, If(Err().Op("!=").Nil()).Block(
		Return(Lit(0), Err()),
	))
	decCodes = append(decCodes, Id(decodeChildValue).Op("=").Make(ast.TypeJenChain(), Id(decodeChildValue+"l")))

	da := []Code{ast.Key.TypeJenChain(Var().Id(decodeChildKey + "v"))}
	da = append(da, daKey...)
	da = append(da, ast.Value.TypeJenChain(Var().Id(decodeChildValue+"v")))
	da = append(da, daValue...)
	da = append(da, Id(decodeChildValue).Index(Id(decodeChildKey+"v")).Op("=").Id(decodeChildValue+"v"))

	decCodes = append(decCodes, For(Id(decodeChildValue+"i").Op(":=").Lit(0).Op(";").Id(decodeChildValue+"i").Op("<").Id(decodeChildValue+"l").Op(";").Id(decodeChildValue+"i").Op("++")).Block(
		da...,
	))
	decCodes = append(decCodes, Id(decodeFieldName).Op("=").Op(andOp).Id(decodeChildValue))

	dArray = append(dArray, If(Op("!").Id(idDecoder).Dot("IsCodeNil").Call(Id("offset"))).Block(
		decCodes...,
	).Else().Block(
		Id("offset").Op("++"),
	))

	return cArray, cArray, eArray, eArray, dArray, dArray, nil
}

func (as *analyzedStruct) createSliceCode(ast *analyzedASTFieldType, encodeFieldName, decodeFieldName string) (cArray []Code, cMap []Code, eArray []Code, eMap []Code, dArray []Code, dMap []Code, err error) {

	encodeChildName, decodeChildName := encodeFieldName+"v", decodeFieldName+""
	if isRootField(encodeFieldName) {
		encodeChildName = "vv"
	}
	if isRootField(decodeFieldName) {
		decodeChildName = "vv"
	}

	decodeChildLengthName := decodeChildName + "l"
	decodeChildIndexName := decodeChildName + "i"
	decodeChildChildName := decodeChildName + "v"

	ca, _, ea, _, da, _, _ := as.createFieldCode(ast.Elm(), encodeChildName, decodeChildName)
	isChildByte := ast.Elm().IsIdentical() && ast.Elm().IdenticalName == "byte"

	calcCodes := as.addSizePattern2("CalcSliceLength", Len( /*Op(ptrOp).*/ Id(encodeFieldName)), Lit(isChildByte))
	calcCodes = append(calcCodes, For(List(Id("_"), Id(encodeChildName)).Op(":=").Range(). /*Op(ptrOp).*/ Id(encodeFieldName)).Block(
		ca...,
	))

	cArray = append(cArray, If( /*Op(ptrOp).*/ Id(encodeFieldName).Op("!=").Nil()).Block(
		calcCodes...,
	).Else().Block(
		as.addSizePattern1("CalcNil"),
	))

	encCodes := make([]Code, 0)
	encCodes = append(encCodes, Id("offset").Op("=").Id(idEncoder).Dot("WriteSliceLength").Call(Len( /*Op(ptrOp).*/ Id(encodeFieldName)), Id("offset"), Lit(isChildByte)))
	encCodes = append(encCodes, For(List(Id("_"), Id(encodeChildName)).Op(":=").Range(). /*Op(ptrOp).*/ Id(encodeFieldName)).Block(
		ea...,
	))

	eArray = append(eArray, If( /*Op(ptrOp).*/ Id(encodeFieldName).Op("!=").Nil()).Block(
		encCodes...,
	).Else().Block(
		Id("offset").Op("=").Id(idEncoder).Dot("WriteNil").Call(Id("offset")),
	))

	decCodes := make([]Code, 0)
	decCodes = append(decCodes, ast.TypeJenChain(Var().Id(decodeChildName)))
	decCodes = append(decCodes, Var().Id(decodeChildLengthName).Int())
	decCodes = append(decCodes, List(Id(decodeChildLengthName), Id("offset"), Err()).Op("=").Id(idDecoder).Dot("SliceLength").Call(Id("offset")))
	decCodes = append(decCodes, If(Err().Op("!=").Nil()).Block(
		Return(Lit(0), Err()),
	))
	decCodes = append(decCodes, Id(decodeChildName).Op("=").Make(ast.TypeJenChain(), Id(decodeChildLengthName)))

	da = append([]Code{ast.Elm().TypeJenChain(Var().Id(decodeChildChildName))}, da...)
	da = append(da, Id(decodeChildName).Index(Id(decodeChildIndexName)).Op("=").Id(decodeChildChildName))

	decCodes = append(decCodes, For(Id(decodeChildIndexName).Op(":=").Range().Id(decodeChildName)).Block(
		da...,
	))

	// todo : 不要なコードがあるはず
	ptrOp := ""
	andOp := ""
	prtCount := 0
	node := ast
	for {
		if node.HasParent() && node.Parent.IsPointer() {
			ptrOp += "*"
			andOp += "&"
			prtCount++
			node = node.Parent
		} else {
			break
		}
	}

	name := decodeChildName
	if prtCount > 0 {
		andOp = "&"
	}
	for i := 0; i < prtCount-1; i++ {
		n := "_" + name
		decCodes = append(decCodes, Id(n).Op(":=").Op("&").Id(name))
		name = n
	}

	// todo ; ここのandOP
	decCodes = append(decCodes, Id(decodeFieldName).Op("=").Op(andOp).Id(name))

	// todo : ようかくにん、重複コードをスキップ
	if ast.HasParent() && ast.Parent.IsPointer() {
		dArray = decCodes
	} else {

		dArray = append(dArray, If(Op("!").Id(idDecoder).Dot("IsCodeNil").Call(Id("offset"))).Block(
			decCodes...,
		).Else().Block(
			Id("offset").Op("++"),
		))
	}

	return cArray, cArray, eArray, eArray, dArray, dArray, nil
}

func (as *analyzedStruct) createArrayCode(ast *analyzedASTFieldType, encodeFieldName, decodeFieldName string) (cArray []Code, cMap []Code, eArray []Code, eMap []Code, dArray []Code, dMap []Code, err error) {

	encodeChildName := encodeFieldName + "v"
	if isRootField(encodeFieldName) {
		encodeChildName = "vv"
	}

	decodeChildName := decodeFieldName + "v"
	if isRootField(decodeFieldName) {
		decodeChildName = "vv"
	}

	//ptrOp := ""
	andOp := ""
	node := ast
	for {
		if node.HasParent() && node.Parent.IsPointer() {
			//ptrOp += "*"
			andOp += "&"
			node = node.Parent
		} else {
			break
		}
	}

	ca, _, ea, _, da, _, _ := as.createFieldCode(ast.Elm(), encodeChildName, decodeChildName)
	isChildByte := ast.Elm().IsIdentical() && ast.Elm().IdenticalName == "byte"

	calcCodes := as.addSizePattern2("CalcSliceLength", Len( /*Op(ptrOp).*/ Id(encodeFieldName)), Lit(isChildByte))
	calcCodes = append(calcCodes, For(List(Id("_"), Id(encodeChildName)).Op(":=").Range(). /*Op(ptrOp).*/ Id(encodeFieldName)).Block(
		ca...,
	))

	cArray = append(cArray /* If(Op(ptrOp).Id(name).Op("!=").Nil()).*/, Block(
		calcCodes...,
	), /*.Else().Block(
		as.addSizePattern1("CalcNil"),
	)*/)

	encCodes := make([]Code, 0)
	encCodes = append(encCodes, Id("offset").Op("=").Id(idEncoder).Dot("WriteSliceLength").Call(Len( /*Op(ptrOp).*/ Id(encodeFieldName)), Id("offset"), Lit(isChildByte)))
	encCodes = append(encCodes, For(List(Id("_"), Id(encodeChildName)).Op(":=").Range(). /*Op(ptrOp).*/ Id(encodeFieldName)).Block(
		ea...,
	))

	eArray = append(eArray /*If(Op(ptrOp).Id(name).Op("!=").Nil()).*/, Block(
		encCodes...,
	), /*.Else().Block(
		Id("offset").Op("=").Id(idEncoder).Dot("WriteNil").Call(Id("offset")),
	)*/)

	decCodes := make([]Code, 0)
	decCodes = append(decCodes, ast.TypeJenChain(Var().Id(decodeChildName)))
	decCodes = append(decCodes, Var().Id(decodeChildName+"l").Int())
	decCodes = append(decCodes, List(Id(decodeChildName+"l"), Id("offset"), Err()).Op("=").Id(idDecoder).Dot("SliceLength").Call(Id("offset")))
	decCodes = append(decCodes, If(Err().Op("!=").Nil()).Block(
		Return(Lit(0), Err()),
	))
	decCodes = append(decCodes, If(Id(decodeChildName+"l").Op(">").Id(fmt.Sprint(ast.ArrayLen))).Block(
		Return(Lit(0), Qual("fmt", "Errorf").Call(Lit("length size(%d) is over array size(%d)"), Id(decodeChildName+"l"), Id(fmt.Sprint(ast.ArrayLen)))),
	))

	da = append([]Code{ast.Elm().TypeJenChain(Var().Id(decodeChildName + "v"))}, da...)
	da = append(da, Id(decodeChildName).Index(Id(decodeChildName+"i")).Op("=").Id(decodeChildName+"v"))

	decCodes = append(decCodes, For(Id(decodeChildName+"i").Op(":=").Range().Id(decodeChildName).Index(Id(":"+decodeChildName+"l"))).Block(
		da...,
	))
	decCodes = append(decCodes, Id(decodeFieldName).Op("=").Op(andOp).Id(decodeChildName))

	// todo : ようかくにん、重複コードをスキップ
	if ast.HasParent() && ast.Parent.IsPointer() {
		dArray = decCodes
	} else {

		dArray = append(dArray, If(Op("!").Id(idDecoder).Dot("IsCodeNil").Call(Id("offset"))).Block(
			decCodes...,
		).Else().Block(
			Id("offset").Op("++"),
		))
	}

	return cArray, cArray, eArray, eArray, dArray, dArray, nil
}

func (as *analyzedStruct) createBasicCode(ast *analyzedASTFieldType, encodeFieldName, decodeFieldName string) (cArray []Code, cMap []Code, eArray []Code, eMap []Code, dArray []Code, dMap []Code, err error) {

	funcSuffix := strings.Title(ast.IdenticalName)

	cArray = append(cArray, as.addSizePattern1("Calc"+funcSuffix, Id(encodeFieldName)))
	eArray = append(eArray, as.encPattern1("Write"+funcSuffix, Id(encodeFieldName), Id("offset")))

	cMap = append(cMap, as.addSizePattern1("Calc"+funcSuffix, Id(encodeFieldName)))
	eMap = append(eMap, as.encPattern1("Write"+funcSuffix, Id(encodeFieldName), Id("offset")))

	dArray = append(dArray, as.decodeBasicPattern(ast, decodeFieldName, "offset", "As"+funcSuffix)...)
	dMap = append(dMap, as.decodeBasicPattern(ast, decodeFieldName, "offset", "As"+funcSuffix)...)

	return cArray, cMap, eArray, eMap, dArray, dMap, err
}

func (as *analyzedStruct) addSizePattern1(funcName string, params ...Code) Code {
	return Id("size").Op("+=").Id(idEncoder).Dot(funcName).Call(params...)
}

func (as *analyzedStruct) addSizePattern2(funcName string, params ...Code) []Code {
	return []Code{
		List(Id("s"), Err()).Op(":=").Id(idEncoder).Dot(funcName).Call(params...),
		If(Err().Op("!=").Nil()).Block(
			Return(Lit(0), Err()),
		),
		Id("size").Op("+=").Id("s"),
	}

}

func (as *analyzedStruct) encPattern1(funcName string, params ...Code) Code {
	return Id("offset").Op("=").Id(idEncoder).Dot(funcName).Call(params...)
}

func isRootField(name string) bool {
	return strings.Contains(name, ".")
}

func (as *analyzedStruct) decodeBasicPattern(ast *analyzedASTFieldType, fieldName, offsetName, decoderFuncName string) []Code {

	varName := fieldName + "v"
	if isRootField(fieldName) {
		varName = "vv"
	}

	node := ast
	ptrCount := 0
	isParentTypeArrayOrMap := false

	for {
		if node.HasParent() {
			node = node.Parent
			if node.IsPointer() {
				ptrCount++
			} else if node.IsSlice() || node.IsArray() || node.IsMap() {
				isParentTypeArrayOrMap = true
				break
			} else {
				// todo : error or empty
			}
		} else {
			break
		}
	}

	codes := make([]Code, 0)
	recieverName := varName

	if ptrCount < 1 && !isParentTypeArrayOrMap {
		codes = append(codes, ast.TypeJenChain(Var().Id(recieverName)))
	} else if isParentTypeArrayOrMap {

		for i := 0; i < ptrCount; i++ {
			p := strings.Repeat("p", i+1)
			kome := strings.Repeat("*", ptrCount-1-i)
			codes = append(codes, ast.TypeJenChain(Var().Id(varName+p).Op(kome)))
		}
		recieverName = varName + strings.Repeat("p", ptrCount)
	} else {
		for i := 0; i < ptrCount; i++ {
			p := strings.Repeat("p", i)
			kome := strings.Repeat("*", ptrCount-1-i)
			codes = append(codes, ast.TypeJenChain(Var().Id(varName+p).Op(kome)))
		}
		recieverName = varName + strings.Repeat("p", ptrCount-1)
	}

	codes = append(codes,
		List(Id(recieverName), Id(offsetName), Err()).Op("=").Id(idDecoder).Dot(decoderFuncName).Call(Id(offsetName)),
		If(Err().Op("!=").Nil()).Block(
			Return(Lit(0), Err()),
		),
	)

	codes = as.createDecodeSetVarPattern(ptrCount, varName, fieldName /*setVarName*/, isParentTypeArrayOrMap, codes)

	// array or map
	if isParentTypeArrayOrMap {
		return codes
	}

	//for i := 0; i < ptrCount; i++ {
	//	if i != ptrCount-1 {
	//		tmp1 := varName + strings.Repeat("p", i)
	//		tmp2 := varName + strings.Repeat("p", i+1)
	//		commons = append(commons, Id(tmp2).Op("=").Op("&").Id(tmp1))
	//	} else {
	//		// last
	//		tmp := varName + strings.Repeat("p", i)
	//		commons = append(commons, Id(setVarName).Op("=").Op("&").Id(tmp))
	//	}
	//}
	//if ptrCount < 1 {
	//	commons = append(commons, Id(setVarName).Op("=").Op("").Id(varName))
	//}
	return []Code{Block(codes...)}
}

func (as *analyzedStruct) createDecodeSetVarPattern(ptrCount int, varName, setVarName string, isLastSkip bool, codes []Code) []Code {

	if isLastSkip {
		for i := 0; i < ptrCount; i++ {
			tmp1 := varName + strings.Repeat("p", ptrCount-1-i)
			tmp2 := varName + strings.Repeat("p", ptrCount-i)
			codes = append(codes, Id(tmp1).Op("=").Op("&").Id(tmp2))
		}
	} else {

		for i := 0; i < ptrCount; i++ {
			if i != ptrCount-1 {
				tmp1 := varName + strings.Repeat("p", ptrCount-2+i)
				tmp2 := varName + strings.Repeat("p", ptrCount-1+i)
				codes = append(codes, Id(tmp1).Op("=").Op("&").Id(tmp2))
			} else {
				// last
				tmp := varName + strings.Repeat("p", 0)
				codes = append(codes, Id(setVarName).Op("=").Op("&").Id(tmp))
			}
		}
		if ptrCount < 1 {
			codes = append(codes, Id(setVarName).Op("=").Op("").Id(varName))
		}
	}

	return codes
}

func (as *analyzedStruct) createNamedCode(encodeFieldName, decodeFieldName string, ast *analyzedASTFieldType) (cArray []Code, cMap []Code, eArray []Code, eMap []Code, dArray []Code, dMap []Code) {

	sizeName := "size_" + encodeFieldName
	if isRootField(encodeFieldName) {
		sizeName = strings.ReplaceAll(sizeName, ".", "_")
	}

	cArray = []Code{
		List(Id(sizeName), Err()).
			Op(":=").
			Id(createFuncName("calcArraySize", ast.StructName, ast.ImportPath)).Call(Id(encodeFieldName), Id(idEncoder)),
		If(Err().Op("!=").Nil()).Block(
			Return(Lit(0), Err()),
		),
		Id("size").Op("+=").Id(sizeName),
	}

	cMap = []Code{
		List(Id(sizeName), Err()).
			Op(":=").
			Id(createFuncName("calcMapSize", ast.StructName, ast.ImportPath)).Call(Id(encodeFieldName), Id(idEncoder)),
		If(Err().Op("!=").Nil()).Block(
			Return(Lit(0), Err()),
		),
		Id("size").Op("+=").Id(sizeName),
	}

	eArray = []Code{
		List(Id("_"), Id("offset"), Err()).
			Op("=").
			Id(createFuncName("encodeArray", ast.StructName, ast.ImportPath)).Call(Id(encodeFieldName), Id(idEncoder), Id("offset")),
		If(Err().Op("!=").Nil()).Block(
			Return(Nil(), Lit(0), Err()),
		),
	}

	eMap = []Code{
		List(Id("_"), Id("offset"), Err()).
			Op("=").
			Id(createFuncName("encodeMap", ast.StructName, ast.ImportPath)).Call(Id(encodeFieldName), Id(idEncoder), Id("offset")),
		If(Err().Op("!=").Nil()).Block(
			Return(Nil(), Lit(0), Err()),
		),
	}

	//varName, setVarName := as.decodeVarPattern(encodeFieldName, isRoot)

	dArray = append(dArray, as.decodeNamedPattern(ast, decodeFieldName, "decodeArray")...)
	dMap = append(dMap, as.decodeNamedPattern(ast, decodeFieldName, "decodeMap")...)

	//dArray = []Code{
	//	Block(
	//		Var().Id(varName).Qual(ast.ImportPath, ast.StructName),
	//		List(Id("offset"), Err()).Op("=").Id(createFuncName("decodeArray", ast.StructName, ast.ImportPath)).Call(Op("&").Id(varName), Id(idDecoder), Id("offset")),
	//		If(Err().Op("!=").Nil()).Block(
	//			Return(Lit(0), Err()),
	//		),
	//		Id(setVarName).Op("=").Id(varName),
	//	),
	//}
	//
	//// dArrayと一緒
	//dMap = []Code{
	//	Block(
	//		Var().Id(varName).Qual(ast.ImportPath, ast.StructName),
	//		List(Id("offset"), Err()).Op("=").Id(createFuncName("decodeMap", ast.StructName, ast.ImportPath)).Call(Op("&").Id(varName), Id(idDecoder), Id("offset")),
	//		If(Err().Op("!=").Nil()).Block(
	//			Return(Lit(0), Err()),
	//		),
	//		Id(setVarName).Op("=").Id(varName),
	//	),
	//}
	return
}

func (as *analyzedStruct) decodeNamedPattern(ast *analyzedASTFieldType, fieldName, decodeFuncName string) []Code {

	varName := fieldName + "v"
	if isRootField(fieldName) {
		varName = "vv"
	}

	node := ast
	ptrCount := 0
	isParentTypeArrayOrMap := false

	for {
		if node.HasParent() {
			node = node.Parent
			if node.IsPointer() {
				ptrCount++
			} else if node.IsSlice() || node.IsArray() || node.IsMap() {
				isParentTypeArrayOrMap = true
				break
			} else {
				// todo : error or empty
			}
		} else {
			break
		}
	}

	codes := make([]Code, 0)
	recieverName := varName

	if ptrCount < 1 && !isParentTypeArrayOrMap {
		codes = append(codes, ast.TypeJenChain(Var().Id(recieverName)))
	} else if isParentTypeArrayOrMap {

		for i := 0; i < ptrCount; i++ {
			p := strings.Repeat("p", i+1)
			kome := strings.Repeat("*", ptrCount-1-i)

			codes = append(codes, ast.TypeJenChain(Var().Id(varName+p).Op(kome)))
		}
		recieverName = varName + strings.Repeat("p", ptrCount)
	} else {
		for i := 0; i < ptrCount; i++ {
			p := strings.Repeat("p", i)
			kome := strings.Repeat("*", ptrCount-1-i)

			codes = append(codes, ast.TypeJenChain(Var().Id(varName+p).Op(kome)))
		}
		recieverName = varName + strings.Repeat("p", ptrCount-1)
	}

	codes = append(codes,
		List(Id("offset"), Err()).Op("=").Id(createFuncName(decodeFuncName, ast.StructName, ast.ImportPath)).Call(Op("&").Id(recieverName), Id(idDecoder), Id("offset")),
		If(Err().Op("!=").Nil()).Block(
			Return(Lit(0), Err()),
		),
	)

	codes = as.createDecodeSetVarPattern(ptrCount, varName, fieldName /*setVarName*/, isParentTypeArrayOrMap, codes)

	// array or map
	if isParentTypeArrayOrMap {
		return codes
	}

	//for i := 0; i < ptrCount; i++ {
	//	if i != ptrCount-1 {
	//		tmp1 := varName + strings.Repeat("p", i)
	//		tmp2 := varName + strings.Repeat("p", i+1)
	//		commons = append(commons, Id(tmp2).Op("=").Op("&").Id(tmp1))
	//	} else {
	//		// last
	//		tmp := varName + strings.Repeat("p", i)
	//		commons = append(commons, Id(setVarName).Op("=").Op("&").Id(tmp))
	//	}
	//}
	//if ptrCount < 1 {
	//	commons = append(commons, Id(setVarName).Op("=").Op("").Id(varName))
	//}
	return []Code{Block(codes...)}
}

func (as *analyzedStruct) decodeVarPattern(fieldName string, isRoot bool) (varName string, setVarName string) {

	varName = "vv"
	setVarName = "v." + fieldName
	if !isRoot {
		varName = fieldName + "v"
		setVarName = fieldName
	}
	return
}
