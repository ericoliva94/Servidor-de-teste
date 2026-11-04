"""Gera as structs Go a partir dos records Delphi resolvidos."""
import os, re, sys
import parse_pas as P
import resolve as R
from parse_pas import is_op

OUT = '/home/user/Servidor-de-teste/GoServer'

# tipos ja escritos a mao: nome Delphi -> (pacote, nome Go)
HANDWRITTEN = {
    'titem':       ('model', 'Item'),
    'titemeffect': ('model', 'ItemEffect'),
    'tposition':   ('model', 'Position'),
}

# unit de origem -> pacote Go
UNIT_PKG = {
    'MiscData.pas':     'model',
    'PlayerData.pas':   'model',
    'GuildData.pas':    'model',
    'PartyData.pas':    'model',
    'EntityMail.pas':   'model',
    'EntityFriend.pas': 'model',
    'FilesData.pas':    'gamedata',
    'Packets.pas':      'packets',
}

def go_name(delphi):
    n = delphi
    if len(n) > 1 and n[0] in 'Tt' and n[1].isupper():
        n = n[1:]
    n = re.sub(r'[^0-9A-Za-z_]', '_', n)
    if not n or not n[0].isalpha():
        n = 'X' + n
    return n[0].upper() + n[1:]

def go_field(name, used):
    f = re.sub(r'[^0-9A-Za-z_]', '_', name)
    if not f or not f[0].isalpha():
        f = 'F' + f
    f = f[0].upper() + f[1:]
    base, i = f, 2
    while f in used:
        f = f'{base}{i}'; i += 1
    used.add(f)
    return f

class Emitter:
    def __init__(s, reg, sizes):
        s.reg, s.sizes = reg, sizes
        s.goname = {}
        s.pkg_of = {}
        s.skipped = []
        for low, rec in reg.recs.items():
            if low in HANDWRITTEN:
                s.pkg_of[low], s.goname[low] = HANDWRITTEN[low]
                continue
            pkg = UNIT_PKG.get(rec.unit)
            if pkg is None:
                continue
            s.pkg_of[low] = pkg
            s.goname[low] = go_name(rec.name)

    def gotype_tokens(s, tt, pkg, stack=()):
        """devolve o tipo Go de um campo"""
        t0 = tt[0]
        if is_op(t0, '^'):
            return 'uint64'
        lw = t0.low if t0.kind == 'id' else ''
        if lw == 'packed':
            return s.gotype_tokens(tt[1:], pkg, stack)
        if lw == 'array':
            d = 0; of = None
            for i, t in enumerate(tt):
                if t.kind == 'op':
                    if t.val in '([': d += 1
                    elif t.val in ')]': d -= 1
                elif t.kind == 'id' and t.low == 'of' and d == 0:
                    of = i; break
            if of is None or of == 1:
                return 'uint64'
            counts = s.array_counts(tt[1:of])
            inner = s.gotype_tokens(tt[of+1:], pkg, stack)
            for c in reversed(counts):
                inner = f'[{c}]' + inner
            return inner
        if lw == 'string':
            nums = [t for t in tt if t.kind == 'num']
            return f'[{int(nums[0].val)+1}]byte'
        return s.gotype_name(lw, pkg, stack)

    def array_counts(s, dims):
        inner = dims
        if inner and is_op(inner[0], '['): inner = inner[1:]
        if inner and is_op(inner[-1], ']'): inner = inner[:-1]
        ranges, cur, d = [], [], 0
        for t in inner:
            if t.kind == 'op':
                if t.val in '([': d += 1
                elif t.val in ')]': d -= 1
                elif t.val == ',' and d == 0:
                    ranges.append(cur); cur = []; continue
            cur.append(t)
        if cur: ranges.append(cur)
        return [s.reg.range_count(r) for r in ranges]

    def gotype_name(s, low, pkg, stack=()):
        if low in P.SCALARS:
            return P.SCALARS[low][1]
        if low in s.reg.enums:
            e = s.reg.enums[low]
            return {1: 'uint8', 2: 'uint16', 4: 'uint32'}[e.size]
        if low in s.goname:
            target_pkg, name = s.pkg_of[low], s.goname[low]
            return name if target_pkg == pkg else f'{target_pkg}.{name}'
        if low in s.reg.aliases:
            return s.gotype_tokens(s.reg.aliases[low].typ, pkg, stack + (low,))
        raise R.Unresolved(low)

    def emit_rec(s, rec, pkg, out, used_names=None):
        low = rec.name.lower()
        lines = []
        size = s.sizes[low]
        lines.append(f'// {s.goname[low]} e {rec.name} de {rec.unit}'
                     f' ({"packed " if rec.packed else ""}record, {size} bytes).')
        if not rec.packed:
            lines.append('//')
            lines.append('// Record sem "packed": os campos _pad reproduzem o')
            lines.append('// alinhamento que o Delphi insere, para que o layout')
            lines.append('// empacotado do Go case com o da origem.')
        lines.append(f'type {s.goname[low]} struct {{')
        used = set()
        off = 0
        npad = 0
        for f in rec.fields:
            if f.typ[0] == 'inline':
                sub = f.typ[1]
                subname = s.goname[low] + go_field(f.name, set())
                s.goname[sub.name.lower()] = subname
                s.pkg_of[sub.name.lower()] = pkg
                s.sizes[sub.name.lower()] = s.reg.size_of_rec(sub)[0]
                out.setdefault(pkg, []).append(s.emit_rec(sub, pkg, out))
                gt = subname
                fsize = s.sizes[sub.name.lower()]
                falign = s.reg.align_of_rec(sub)
            else:
                gt = s.gotype_tokens(f.typ[1], pkg)
                fsize, _ = s.reg.size_of_tokens(f.typ[1])
                falign = s.reg.align_of_tokens(f.typ[1])
            if not rec.packed:
                a = min(falign, s.reg.maxalign)
                if a > 1 and off % a:
                    pad = a - (off % a)
                    npad += 1
                    lines.append(f'\t_ [{pad}]byte // alinhamento do Delphi')
                    off += pad
            lines.append(f'\t{go_field(f.name, used)} {gt}')
            off += fsize
        if not rec.packed:
            ra = s.reg.align_of_rec(rec)
            if ra > 1 and off % ra:
                pad = ra - (off % ra)
                npad += 1
                lines.append(f'\t_ [{pad}]byte // cauda alinhada pelo Delphi')
                off += pad
        lines.append('}')
        return '\n'.join(lines)

def main():
    reg, sizes, bad = R.resolve_all(8)
    em = Emitter(reg, sizes)
    out, tests, skipped = {}, {}, []

    for low, rec in sorted(reg.recs.items(), key=lambda kv: kv[1].name.lower()):
        if low in HANDWRITTEN:    continue
        if low not in em.pkg_of:  continue
        pkg = em.pkg_of[low]
        if low not in sizes:
            skipped.append((rec.name, rec.unit, bad_reason(bad, rec.name))); continue
        if rec.variant:
            skipped.append((rec.name, rec.unit, 'record com parte variante')); continue
        try:
            code = em.emit_rec(rec, pkg, out)
        except R.Unresolved as e:
            skipped.append((rec.name, rec.unit, f'tipo de campo: {e}')); continue
        out.setdefault(pkg, []).append(code)
        tests.setdefault(pkg, []).append((em.goname[low], sizes[low], not rec.packed))

    for pkg, blocks in out.items():
        d = os.path.join(OUT, 'internal', pkg)
        os.makedirs(d, exist_ok=True)
        hdr = [f'// Code generated by gen/emit.py from Server/Src. DO NOT EDIT.',
               '', f'package {pkg}', '']
        if pkg != 'model' and any('model.' in b for b in blocks):
            hdr += ['import "aika/internal/model"', '']
        open(os.path.join(d, 'types_gen.go'), 'w').write('\n'.join(hdr + blocks) + '\n')

        tl = [f'// Code generated by gen/emit.py from Server/Src. DO NOT EDIT.',
              '', f'package {pkg}', '',
              'import (', '\t"bytes"', '\t"encoding/binary"', '\t"reflect"',
              '\t"testing"', ')', '',
              'type genCase struct {',
              '\tname   string',
              '\tv      any',
              '\twant   int',
              '\thasPad bool',
              '}', '',
              'func genCases() []genCase {',
              '\treturn []genCase{']
        for name, size, haspad in sorted(tests.get(pkg, [])):
            tl.append(f'\t\t{{"{name}", {name}{{}}, {size}, {str(haspad).lower()}}},')
        tl += ['\t}', '}', '',
               '// O layout binario tem que bater com o sizeof() do Delphi: essas',
               '// structs viajam na rede e vao para o banco.',
               'func TestGeneratedSizesMatchDelphi(t *testing.T) {',
               '\tfor _, c := range genCases() {',
               '\t\tif got := binary.Size(c.v); got != c.want {',
               '\t\t\tt.Errorf("%s: %d bytes, o Delphi tem %d", c.name, got, c.want)',
               '\t\t}', '\t}', '}', '',
               '// binary.Size nao exercita a reflexao de escrita: um campo de',
               '// padding com nome nao exportado diferente de "_" faz binary.Read',
               '// entrar em panico. Este teste decodifica e recodifica cada struct',
               '// para pegar isso.',
               'func TestGeneratedRoundTrip(t *testing.T) {',
               '\tfor _, c := range genCases() {',
               '\t\tsize := binary.Size(c.v)',
               '\t\tbuf := make([]byte, size)',
               '\t\tfor i := range buf {',
               '\t\t\tbuf[i] = byte(i*31 + 7)',
               '\t\t}',
               '\t\tptr := reflect.New(reflect.TypeOf(c.v))',
               '\t\tif err := binary.Read(bytes.NewReader(buf), binary.LittleEndian, ptr.Interface()); err != nil {',
               '\t\t\tt.Errorf("%s: Read: %v", c.name, err)',
               '\t\t\tcontinue',
               '\t\t}',
               '\t\tvar out bytes.Buffer',
               '\t\tif err := binary.Write(&out, binary.LittleEndian, ptr.Elem().Interface()); err != nil {',
               '\t\t\tt.Errorf("%s: Write: %v", c.name, err)',
               '\t\t\tcontinue',
               '\t\t}',
               '\t\tif out.Len() != size {',
               '\t\t\tt.Errorf("%s: recodificou %d bytes, queria %d", c.name, out.Len(), size)',
               '\t\t\tcontinue',
               '\t\t}',
               '\t\t// Sem padding o conteudo tem que voltar identico. Com padding os',
               '\t\t// bytes de alinhamento voltam zerados, entao so o tamanho vale.',
               '\t\tif !c.hasPad && !bytes.Equal(out.Bytes(), buf) {',
               '\t\t\tt.Errorf("%s: ida e volta mudou o conteudo", c.name)',
               '\t\t}',
               '\t}', '}']
        open(os.path.join(d, 'types_gen_test.go'), 'w').write('\n'.join(tl) + '\n')
        print(f'{pkg}: {len(tests.get(pkg,[]))} structs')

    with open(os.path.join(OUT, 'gen', 'NAO_PORTADO.md'), 'w') as f:
        f.write('# Records nao gerados automaticamente\n\n')
        f.write('Gerado por `gen/emit.py`. Cada um precisa de tipo Go escrito a\n')
        f.write('mao porque nao tem layout binario fixo (lista, ponteiro, classe)\n')
        f.write('ou tem parte variante.\n\n')
        f.write('| Record | Unit | Motivo |\n|---|---|---|\n')
        for name, unit, why in sorted(skipped):
            f.write(f'| `{name}` | `{unit}` | {why} |\n')
    print(f'nao gerados: {len(skipped)} (ver gen/NAO_PORTADO.md)')

def bad_reason(bad, name):
    for why, names in bad.items():
        if name in names: return why
    return 'nao resolvido'

if __name__ == '__main__':
    main()
