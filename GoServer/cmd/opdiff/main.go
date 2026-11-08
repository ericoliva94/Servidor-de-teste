// Comando opdiff compara os codigos de pacote tratados pelo servidor Delphi
// (o `case Header.Code of` de ServerSocket.pas e LoginSocket.pas) com os
// codigos que o Go conhece (constantes `Code* = 0x...` e `case 0x...:`).
//
// E uma ferramenta de auditoria: diz quais opcodes da origem ainda nao tem
// contraparte no Go. "Tem contraparte" significa apenas que o numero aparece
// no codigo Go; nao prova que o comportamento foi validado.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	labelRe   = regexp.MustCompile(`^\s{2,6}((?:\$[0-9A-Fa-f]+\s*,\s*)*\$[0-9A-Fa-f]+)\s*:\s*(.*)$`)
	handlerRe = regexp.MustCompile(`(T\w+Handlers?\.\w+|T\w+Functions?\.\w+|Player\.\w+)`)
	goConstRe = regexp.MustCompile(`\bCode\w*\s*(?:uint16|uint32|int)?\s*=\s*0x([0-9A-Fa-f]+)`)
	goCaseRe  = regexp.MustCompile(`case\s+(0x[0-9A-Fa-f]+(?:\s*,\s*0x[0-9A-Fa-f]+)*)\s*:`)
)

// delphiOpcodes le o arquivo a partir da linha start (1-based) e devolve
// opcode -> handlers citados no ramo.
func delphiOpcodes(path string, start int) (map[uint32][]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(raw), "\n")
	if start > len(lines) {
		start = len(lines)
	}
	res := map[uint32][]string{}
	var cur []uint32
	for _, ln := range lines[start-1:] {
		if m := labelRe.FindStringSubmatch(ln); m != nil {
			cur = cur[:0]
			for _, p := range strings.Split(m[1], ",") {
				v, err := strconv.ParseUint(strings.TrimSpace(p)[1:], 16, 32)
				if err == nil {
					cur = append(cur, uint32(v))
					if _, ok := res[uint32(v)]; !ok {
						res[uint32(v)] = nil
					}
				}
			}
			ln = m[2]
		}
		for _, h := range handlerRe.FindAllString(ln, -1) {
			for _, c := range cur {
				dup := false
				for _, x := range res[c] {
					dup = dup || x == h
				}
				if !dup {
					res[c] = append(res[c], h)
				}
			}
		}
	}
	return res, nil
}

func goOpcodes(root string) (map[uint32]bool, error) {
	set := map[uint32]bool{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range goConstRe.FindAllStringSubmatch(string(b), -1) {
			v, _ := strconv.ParseUint(m[1], 16, 32)
			set[uint32(v)] = true
		}
		for _, m := range goCaseRe.FindAllStringSubmatch(string(b), -1) {
			for _, x := range strings.Split(m[1], ",") {
				v, err := strconv.ParseUint(strings.TrimSpace(x)[2:], 16, 32)
				if err == nil {
					set[uint32(v)] = true
				}
			}
		}
		return nil
	})
	return set, err
}

func report(title string, d map[uint32][]string, g map[uint32]bool) {
	var miss []uint32
	done := 0
	for c := range d {
		if g[c] {
			done++
		} else {
			miss = append(miss, c)
		}
	}
	sort.Slice(miss, func(i, j int) bool { return miss[i] < miss[j] })
	fmt.Printf("## %s: %d opcodes Delphi, %d com contraparte Go, %d sem\n\n", title, len(d), done, len(miss))
	for _, c := range miss {
		fmt.Printf("- `0x%X` — %s\n", c, strings.Join(d[c], ", "))
	}
	fmt.Println()
}

func main() {
	root := flag.String("root", "..", "raiz do repositorio (pasta que contem Server e GoServer)")
	chStart := flag.Int("ch-start", 2511, "linha do case Header.Code em ServerSocket.pas")
	lgStart := flag.Int("lg-start", 186, "linha do case Header.Code em LoginSocket.pas")
	flag.Parse()

	g, err := goOpcodes(filepath.Join(*root, "GoServer", "internal"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ch, err := delphiOpcodes(filepath.Join(*root, "Server", "Src", "Connections", "ServerSocket.pas"), *chStart)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	lg, err := delphiOpcodes(filepath.Join(*root, "Server", "Src", "Connections", "LoginSocket.pas"), *lgStart)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Constantes/cases Go: %d\n\n", len(g))
	report("Canal (ServerSocket.pas)", ch, g)
	report("Login (LoginSocket.pas)", lg, g)
}
