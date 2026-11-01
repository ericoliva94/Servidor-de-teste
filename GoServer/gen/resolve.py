"""Resolve o layout binario dos records e emite Go."""
import re, os, sys
import parse_pas as P
from parse_pas import SCALARS, Rec, Alias, Enum, is_op, is_id

UNITS = [
    'Server/Src/Data/MiscData.pas',
    'Server/Src/Data/FilesData.pas',
    'Server/Src/Data/PlayerData.pas',
    'Server/Src/Guild/GuildData.pas',
    'Server/Src/Party/PartyData.pas',
    'Server/Src/Data/Entity/EntityMail.pas',
    'Server/Src/Data/Entity/EntityFriend.pas',
    'Server/Src/Data/Packets.pas',
]

class Registry:
    def __init__(s):
        s.recs, s.aliases, s.enums = {}, {}, {}
        s.sizes = {}
        s.errors = []
    def load(s):
        for u in UNITS:
            minenum = 4 if re.search(r'\{\$MINENUMSIZE\s+4\}',
                      open(os.path.join(P.ROOT,u),'rb').read().decode('utf-8','replace'),re.I) else 1
            r,a,e = P.parse_unit(u, minenum)
            # a primeira declaracao ganha (ordem de UNITS = ordem de dependencia)
            for k,v in r.items(): s.recs.setdefault(k,v)
            for k,v in a.items(): s.aliases.setdefault(k,v)
            for k,v in e.items(): s.enums.setdefault(k,v)

    # ---- resolucao de tipo: devolve (size, gotype) ou levanta Unresolved
    def size_of_name(s, low, stack=()):
        if low in SCALARS: return SCALARS[low]
        if low in s.enums:
            e = s.enums[low]
            return (e.size, {1:'uint8',2:'uint16',4:'uint32'}[e.size])
        if low in s.recs:
            if low in stack: raise Unresolved(f'ciclo em {low}')
            return s.size_of_rec(s.recs[low], stack + (low,))
        if low in s.aliases:
            if low in stack: raise Unresolved(f'ciclo em {low}')
            return s.size_of_tokens(s.aliases[low].typ, stack + (low,))
        raise Unresolved(low)

    def size_of_tokens(s, tt, stack=()):
        """tt = lista de tokens do tipo"""
        if not tt: raise Unresolved('tipo vazio')
        t0 = tt[0]
        # ponteiro
        if is_op(t0, '^'): return (8, 'uint64')
        if t0.kind == 'id':
            lw = t0.low
            if lw == 'packed': return s.size_of_tokens(tt[1:], stack)
            if lw == 'array':
                return s.size_of_array(tt, stack)
            if lw == 'string':
                # string[N] = ShortString de N+1 bytes
                m = [t for t in tt if t.kind == 'num']
                if m: n = int(m[0].val); return (n+1, f'[{n+1}]byte')
                raise Unresolved('string sem tamanho (ponteiro)')
            if lw == 'set':
                raise Unresolved('set of')
            if lw in ('tdictionary','tlist','tobjectlist','tstringlist','tthread','tsocket'):
                raise Unresolved(lw)
            if len(tt) == 1 or (len(tt) > 1 and tt[1].kind == 'op' and tt[1].val not in '[.'):
                return s.size_of_name(lw, stack)
            return s.size_of_name(lw, stack)
        raise Unresolved(P.type_tokens_to_str(tt))

    def size_of_array(s, tt, stack=()):
        """array [a..b] of T  |  array [a..b, c..d] of T  |  array of T"""
        # acha o 'of' de topo
        d = 0; of = None
        for i,t in enumerate(tt):
            if t.kind=='op':
                if t.val in '([': d+=1
                elif t.val in ')]': d-=1
            elif t.kind=='id' and t.low=='of' and d==0:
                of = i; break
        if of is None: raise Unresolved('array sem of')
        dims_toks = tt[1:of]
        elem = tt[of+1:]
        if not dims_toks:
            # array of T (dinamico) e um ponteiro de 8 bytes no Win64. Nao e
            # tipo de rede, mas resolver o tamanho deixa os dependentes passar.
            return (8, 'uint64')
        # extrai as faixas dentro dos colchetes
        inner = dims_toks
        if inner and is_op(inner[0],'['): inner = inner[1:]
        if inner and is_op(inner[-1],']'): inner = inner[:-1]
        ranges, cur = [], []
        d = 0
        for t in inner:
            if t.kind=='op':
                if t.val in '([': d+=1
                elif t.val in ')]': d-=1
                elif t.val==',' and d==0:
                    ranges.append(cur); cur=[]; continue
            cur.append(t)
        if cur: ranges.append(cur)
        counts = []
        for r in ranges:
            counts.append(s.range_count(r, stack))
        esize, egotype = s.size_of_tokens(elem, stack)
        total = esize
        for c in counts: total *= c
        go = egotype
        for c in reversed(counts): go = f'[{c}]' + go
        return (total, go)

    def range_count(s, toks, stack=()):
        txt = P.type_tokens_to_str(toks)
        m = re.match(r'^\s*(\S+)\s*\.\.\s*(\S+)\s*$', txt)
        if not m:
            # pode ser um tipo (array[TEnum] of ...) ou uma const
            low = txt.strip().lower()
            if low in s.enums: return len(s.enums[low].members)
            raise Unresolved(f'faixa {txt!r}')
        lo = s.const_val(m.group(1)); hi = s.const_val(m.group(2))
        if hi < lo: raise Unresolved(f'faixa invertida {txt!r}')
        return hi - lo + 1

    CONSTS = {}
    def const_val(s, tok):
        tok = tok.strip()
        if re.fullmatch(r'\$[0-9A-Fa-f]+', tok): return int(tok[1:],16)
        if re.fullmatch(r'-?\d+', tok): return int(tok)
        low = tok.lower()
        if low in s.CONSTS: return s.CONSTS[low]
        raise Unresolved(f'const {tok!r}')

    # ---- alinhamento (records sem 'packed')
    def align_of_tokens(s, tt, stack=()):
        t0 = tt[0]
        if is_op(t0, '^'): return 8
        lw = t0.low if t0.kind == 'id' else ''
        if lw == 'packed': return s.align_of_tokens(tt[1:], stack)
        if lw == 'array':
            d = 0; of = None
            for i, t in enumerate(tt):
                if t.kind == 'op':
                    if t.val in '([': d += 1
                    elif t.val in ')]': d -= 1
                elif t.kind == 'id' and t.low == 'of' and d == 0:
                    of = i; break
            if of is None: raise Unresolved('array sem of')
            if of == 1: return 8   # dinamico = ponteiro
            return s.align_of_tokens(tt[of+1:], stack)
        if lw == 'string': return 1
        return s.align_of_name(lw, stack)

    def align_of_name(s, low, stack=()):
        if low in SCALARS:
            sz = SCALARS[low][0]
            return min(sz, 8) if sz in (1,2,4,8) else 2
        if low in s.enums: return s.enums[low].size
        if low in s.recs:
            if low in stack: raise Unresolved(f'ciclo em {low}')
            return s.align_of_rec(s.recs[low], stack + (low,))
        if low in s.aliases:
            if low in stack: raise Unresolved(f'ciclo em {low}')
            return s.align_of_tokens(s.aliases[low].typ, stack + (low,))
        raise Unresolved(low)

    def align_of_rec(s, rec, stack=()):
        if rec.packed: return 1
        a = 1
        for f in rec.fields:
            if f.typ[0] == 'inline':
                fa = s.align_of_rec(f.typ[1], stack)
            else:
                fa = s.align_of_tokens(f.typ[1], stack)
            a = max(a, min(fa, s.maxalign))
        return a

    maxalign = 8

    def size_of_rec(s, rec, stack=()):
        if rec.variant:
            raise Unresolved(f'{rec.name}: record com parte variante')
        total = 0
        for f in rec.fields:
            if f.typ[0] == 'inline':
                sz, _ = s.size_of_rec(f.typ[1], stack)
                fa = s.align_of_rec(f.typ[1], stack)
            else:
                sz, _ = s.size_of_tokens(f.typ[1], stack)
                fa = s.align_of_tokens(f.typ[1], stack)
            if not rec.packed:
                a = min(fa, s.maxalign)
                if a > 1 and total % a:
                    total += a - (total % a)
            total += sz
        if not rec.packed:
            ra = s.align_of_rec(rec, stack)
            if ra > 1 and total % ra:
                total += ra - (total % ra)
        return (total, 'struct')

class Unresolved(Exception): pass

def resolve_all(maxalign):
    reg = Registry(); reg.load(); reg.maxalign = maxalign
    sizes, bad = {}, {}
    for k, r in sorted(reg.recs.items()):
        try:
            sizes[k] = reg.size_of_rec(r)[0]
        except Unresolved as e:
            bad.setdefault(str(e), []).append(r.name)
    return reg, sizes, bad

if __name__ == '__main__':
    reg = Registry(); reg.load()
    print(f'records={len(reg.recs)} aliases={len(reg.aliases)} enums={len(reg.enums)}')
    ok, bad = 0, {}
    for k, r in sorted(reg.recs.items()):
        try:
            sz,_ = reg.size_of_rec(r)
            reg.sizes[k] = sz; ok += 1
        except Unresolved as e:
            bad.setdefault(str(e), []).append(r.name)
    print(f'resolvidos={ok}  nao resolvidos={sum(len(v) for v in bad.values())}')
    for motivo, nomes in sorted(bad.items(), key=lambda x:-len(x[1]))[:18]:
        print(f'  {motivo:42} {len(nomes):3}  ex: {", ".join(nomes[:3])}')
    for n in ('tpacketheader','titem','tposition','tsignaldata'):
        if n in reg.sizes: print(f'  {n} = {reg.sizes[n]} bytes')
