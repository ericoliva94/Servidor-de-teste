package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// Named traduz parametros nomeados no estilo do Delphi (":nome") para os
// marcadores posicionais "?" que o driver MySQL usa, devolvendo os valores na
// ordem em que aparecem.
//
// O SQL.pas usava TFDQuery.ParamByName, que aceita ":nome" direto. O
// database/sql nao tem isso para MySQL, e montar a query com concatenacao —
// como boa parte do codigo legado faz — abre injecao de SQL. Esta funcao
// permite manter as queries legiveis e ainda assim passar valor como
// parametro.
//
// Quando a query nao tem nenhum ":nome", os argumentos passam direto: assim o
// estilo posicional com "?" continua funcionando sem tratamento especial.
//
// Os valores podem vir como sql.Named(...) ou como um unico map[string]any.
func Named(query string, args ...any) (string, []any, error) {
	names := scanNames(query)
	if len(names) == 0 {
		return query, args, nil
	}

	values, err := namedValues(args)
	if err != nil {
		return "", nil, err
	}

	var b strings.Builder
	b.Grow(len(query))
	out := make([]any, 0, len(names))

	for _, seg := range splitQuery(query) {
		if seg.name == "" {
			b.WriteString(seg.text)
			continue
		}
		v, ok := values[seg.name]
		if !ok {
			return "", nil, fmt.Errorf("store: parametro :%s nao foi informado", seg.name)
		}
		b.WriteByte('?')
		out = append(out, v)
	}

	// Parametro informado e nao usado costuma ser erro de digitacao no nome,
	// e silenciar isso foi exatamente o que fez o retry do Delphi gravar NULL.
	used := make(map[string]bool, len(names))
	for _, n := range names {
		used[n] = true
	}
	for n := range values {
		if !used[n] {
			return "", nil, fmt.Errorf("store: parametro :%s informado mas nao usado na query", n)
		}
	}

	return b.String(), out, nil
}

func namedValues(args []any) (map[string]any, error) {
	values := map[string]any{}
	if len(args) == 1 {
		if m, ok := args[0].(map[string]any); ok {
			for k, v := range m {
				values[strings.ToLower(k)] = v
			}
			return values, nil
		}
	}
	for i, a := range args {
		na, ok := a.(sql.NamedArg)
		if !ok {
			return nil, fmt.Errorf(
				"store: argumento %d e %T: com :nome na query use sql.Named ou um map[string]any",
				i, a)
		}
		if na.Name == "" {
			return nil, fmt.Errorf("store: argumento %d tem nome vazio", i)
		}
		values[strings.ToLower(na.Name)] = na.Value
	}
	return values, nil
}

type segment struct {
	text string // trecho literal, quando name == ""
	name string // nome do parametro, sem os dois-pontos
}

// splitQuery quebra a query em trechos literais e parametros, ignorando o que
// esta dentro de string, identificador entre acentos graves ou comentario.
func splitQuery(q string) []segment {
	var segs []segment
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			segs = append(segs, segment{text: lit.String()})
			lit.Reset()
		}
	}

	for i := 0; i < len(q); {
		c := q[i]
		switch {
		// string entre apostrofos ou aspas, e identificador entre acentos graves
		case c == '\'' || c == '"' || c == '`':
			j := skipQuoted(q, i)
			lit.WriteString(q[i:j])
			i = j

		// comentario de linha: -- ou #
		case c == '#' || (c == '-' && i+1 < len(q) && q[i+1] == '-'):
			j := strings.IndexByte(q[i:], '\n')
			if j < 0 {
				lit.WriteString(q[i:])
				i = len(q)
			} else {
				lit.WriteString(q[i : i+j+1])
				i += j + 1
			}

		// comentario de bloco
		case c == '/' && i+1 < len(q) && q[i+1] == '*':
			j := strings.Index(q[i+2:], "*/")
			if j < 0 {
				lit.WriteString(q[i:])
				i = len(q)
			} else {
				lit.WriteString(q[i : i+2+j+2])
				i += 2 + j + 2
			}

		case c == ':':
			// "::" e operador de cast em outros bancos: nao e parametro
			if i+1 < len(q) && q[i+1] == ':' {
				lit.WriteString("::")
				i += 2
				continue
			}
			j := i + 1
			for j < len(q) && isNameByte(q[j]) {
				j++
			}
			if j == i+1 { // dois-pontos solto
				lit.WriteByte(c)
				i++
				continue
			}
			flush()
			segs = append(segs, segment{name: strings.ToLower(q[i+1 : j])})
			i = j

		default:
			lit.WriteByte(c)
			i++
		}
	}
	flush()
	return segs
}

// scanNames lista os parametros usados na query, na ordem.
func scanNames(q string) []string {
	var out []string
	for _, s := range splitQuery(q) {
		if s.name != "" {
			out = append(out, s.name)
		}
	}
	return out
}

// skipQuoted devolve o indice do primeiro byte depois do literal que comeca em
// i. Trata o escape por duplicacao (”) e por barra invertida (\'), que e o
// padrao do MySQL.
func skipQuoted(q string, i int) int {
	quote := q[i]
	j := i + 1
	for j < len(q) {
		switch q[j] {
		case '\\':
			if quote != '`' {
				j += 2
				continue
			}
			j++
		case quote:
			if j+1 < len(q) && q[j+1] == quote {
				j += 2
				continue
			}
			return j + 1
		default:
			j++
		}
	}
	return len(q) // literal nao fechado: devolve o resto
}

func isNameByte(c byte) bool {
	return c == '_' ||
		(c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z')
}
