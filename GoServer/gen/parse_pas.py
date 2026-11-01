"""Extrai declaracoes de tipo das units Delphi e resolve o layout binario."""
import sys, os, re
sys.path.insert(0,'/tmp/claude-0/-home-user-Servidor-de-teste/f29312d4-231b-514b-b261-323474d7ed97/scratchpad')
from paslex import tokenize, read_text

ROOT = '/home/user/Servidor-de-teste'

def is_op(t, v): return t is not None and t.kind == 'op' and t.val == v
def is_id(t, *v): return t is not None and t.kind == 'id' and t.low in v

# ---- tipos escalares: nome Delphi -> (tamanho, tipo Go)
SCALARS = {
    'byte': (1, 'uint8'), 'shortint': (1, 'int8'), 'ansichar': (1, 'byte'),
    # Boolean virou uint8 de proposito: binary.Read aceita qualquer byte num
    # bool do Go, mas binary.Write devolve 0x01, e pacote que o servidor
    # ecoa de volta sairia alterado. uint8 preserva o byte como veio.
    'boolean': (1, 'uint8'), 'bytebool': (1, 'uint8'),
    'word': (2, 'uint16'), 'smallint': (2, 'int16'), 'wordbool': (2, 'uint16'),
    'char': (2, 'uint16'), 'widechar': (2, 'uint16'),
    'integer': (4, 'int32'), 'longint': (4, 'int32'), 'int32': (4, 'int32'),
    'cardinal': (4, 'uint32'), 'dword': (4, 'uint32'), 'longword': (4, 'uint32'),
    'uint32': (4, 'uint32'), 'single': (4, 'float32'),
    'longbool': (4, 'uint32'), 'bool': (4, 'uint32'),
    'int64': (8, 'int64'), 'uint64': (8, 'uint64'), 'largeint': (8, 'int64'),
    'double': (8, 'float64'), 'tdatetime': (8, 'float64'),
    'ttime': (8, 'float64'), 'tdate': (8, 'float64'),
    'extended': (10, 'float64'),
    # grafias alternativas que o codigo usa
    'int8': (1, 'int8'), 'uint8': (1, 'uint8'),
    'int16': (2, 'int16'), 'uint16': (2, 'uint16'),
    'nativeint': (8, 'int64'), 'nativeuint': (8, 'uint64'),
    'pointer': (8, 'uint64'), 'real': (8, 'float64'),
    'currency': (8, 'int64'), 'comp': (8, 'float64'),
    'real48': (6, 'float64'),
}

class Field:
    def __init__(s, name, typ): s.name, s.typ = name, typ
    def __repr__(s): return f'{s.name}:{s.typ}'

class Rec:
    def __init__(s, name, packed, unit):
        s.name, s.packed, s.unit = name, packed, unit
        s.fields = []
        s.variant = False

class Alias:
    def __init__(s, name, typ, unit): s.name, s.typ, s.unit = name, typ, unit

class Enum:
    def __init__(s, name, members, unit, size):
        s.name, s.members, s.unit, s.size = name, members, unit, size

def type_tokens_to_str(toks):
    out = []
    for t in toks:
        out.append(t.val)
    return ' '.join(out)

def parse_unit(rel, minenumsize=4):
    """devolve (recs, aliases, enums) declarados na unit"""
    path = os.path.join(ROOT, rel)
    toks, _ = tokenize(read_text(path))
    n = len(toks)
    recs, aliases, enums = {}, {}, {}
    unit = os.path.basename(rel)
    i = 0
    in_type = False
    while i < n:
        t = toks[i]
        if t.kind == 'id':
            lw = t.low
            if lw == 'type':
                in_type = True; i += 1; continue
            if lw in ('var', 'const', 'procedure', 'function', 'implementation',
                      'begin', 'threadvar', 'resourcestring', 'uses', 'interface',
                      'constructor', 'destructor', 'class', 'property'):
                if lw != 'interface' or (i > 0 and is_op(toks[i-1], '=')):
                    in_type = False
            if in_type and t.kind == 'id' and i + 1 < n and is_op(toks[i+1], '='):
                name = t.val
                j = i + 2
                res, j = parse_type_decl(name, toks, j, n, unit, minenumsize)
                if isinstance(res, Rec):      recs[name.lower()] = res
                elif isinstance(res, Alias):  aliases[name.lower()] = res
                elif isinstance(res, Enum):   enums[name.lower()] = res
                i = j
                continue
        i += 1
    return recs, aliases, enums

def skip_to_semicolon(toks, j, n):
    d = 0
    while j < n:
        t = toks[j]
        if t.kind == 'op':
            if t.val in '([': d += 1
            elif t.val in ')]': d -= 1
            elif t.val == ';' and d == 0: return j + 1
        j += 1
    return j

def parse_type_decl(name, toks, j, n, unit, minenumsize):
    t = toks[j] if j < n else None
    # packed record / record
    packed = False
    if is_id(t, 'packed'):
        packed = True; j += 1; t = toks[j] if j < n else None
    if is_id(t, 'record'):
        rec = Rec(name, packed, unit)
        j = parse_record_body(rec, toks, j + 1, n)
        return rec, j
    # enum
    if is_op(t, '('):
        members, d, k = [], 0, j
        while k < n:
            tk = toks[k]
            if tk.kind == 'op':
                if tk.val == '(': d += 1
                elif tk.val == ')':
                    d -= 1
                    if d == 0: break
            elif tk.kind == 'id' and d == 1:
                members.append(tk.val)
            k += 1
        # k aponta para o ')' de fechamento: comeca depois dele, senao a
        # profundidade vira -1 e o laco varre o resto do arquivo.
        return Enum(name, members, unit, minenumsize), skip_to_semicolon(toks, k + 1, n)
    # qualquer outra coisa: guarda os tokens do tipo como alias
    k, d, tt = j, 0, []
    while k < n:
        tk = toks[k]
        if tk.kind == 'op':
            if tk.val in '([': d += 1
            elif tk.val in ')]': d -= 1
            elif tk.val == ';' and d == 0: break
        if tk.kind == 'id' and tk.low == 'record' and d == 0:
            rec = Rec(name, packed, unit)
            k = parse_record_body(rec, toks, k + 1, n)
            return rec, k
        tt.append(tk); k += 1
    return Alias(name, tt, unit), k + 1

def parse_record_body(rec, toks, j, n):
    """le campos ate o 'end' que fecha o record"""
    depth = 1
    while j < n and depth:
        t = toks[j]
        if t.kind == 'id':
            lw = t.low
            if lw == 'end':
                depth -= 1; j += 1; continue
            if lw == 'case':
                rec.variant = True
                j += 1; continue
            if lw in ('private', 'public', 'protected', 'published', 'strict'):
                j += 1; continue
            if lw in ('procedure', 'function', 'constructor', 'destructor',
                      'property', 'class', 'operator'):
                j = skip_member(toks, j, n); continue
            if lw == 'record':
                depth += 1; j += 1; continue
            # campo: nomes separados por virgula, ':' tipo ';'
            names = [t.val]; k = j + 1
            while k < n and is_op(toks[k], ','):
                if k + 1 < n and toks[k+1].kind == 'id':
                    names.append(toks[k+1].val); k += 2
                else: break
            if k < n and is_op(toks[k], ':'):
                k += 1
                tt, d = [], 0
                while k < n:
                    tk = toks[k]
                    if tk.kind == 'op':
                        if tk.val in '([': d += 1
                        elif tk.val in ')]': d -= 1
                        elif tk.val == ';' and d == 0: break
                    if tk.kind == 'id' and tk.low == 'record' and d == 0:
                        sub = Rec(rec.name + '_' + names[0], rec.packed, rec.unit)
                        k = parse_record_body(sub, toks, k + 1, n)
                        for nm in names: rec.fields.append(Field(nm, ('inline', sub)))
                        tt = None
                        break
                    tt.append(tk); k += 1
                if tt is not None:
                    for nm in names: rec.fields.append(Field(nm, ('toks', tt)))
                j = k + 1 if k < n and is_op(toks[k], ';') else k
                continue
            j = k + 1
            continue
        j += 1
    return j

def skip_member(toks, j, n):
    """pula procedure/function/property dentro do record"""
    d = 0
    while j < n:
        t = toks[j]
        if t.kind == 'op':
            if t.val in '([': d += 1
            elif t.val in ')]': d -= 1
            elif t.val == ';' and d == 0: return j + 1
        elif t.kind == 'id' and t.low in ('begin',):
            return j
        j += 1
    return j
