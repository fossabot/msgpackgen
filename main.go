package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/shamaton/msgpackgen/internal/gen"
)

var (
	out     = flag.String("output", "", "output directory")
	input   = flag.String("input", ".", "input directory")
	strict  = flag.Bool("strict", false, "strict mode")
	verbose = flag.Bool("v", false, "verbose diagnostics")
)

func main() {

	flag.Parse()

	_, err := os.Stat(*input)
	if err != nil {
		fmt.Println(err)
		return
	}

	if *out == "" {
		*out = *input
	}

	g := gen.NewGenerator()
	g.Initialize(*input, *out, *strict, *verbose)

	// todo : この呼び方やめる
	files := g.Dirwalk(*input)
	fmt.Println(files)

	// 最初にgenerate対象のパッケージをすべて取得
	// できればコードにエラーがない状態を知りたい

	// todo : 構造体の解析時にgenerate対象でないパッケージを含んだ構造体がある場合
	// 出力対象にしない

	// todo : 出力対象にしない構造体が見つからなくなるまで実行する

	g.GetPackages(files)
	g.CreateAnalyzedStructs()
	g.Generate()
}
