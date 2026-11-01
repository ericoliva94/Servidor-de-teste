# aika_bin_readers.py
# Leitores binários para ItemList.bin e SkillList.bin baseados em FilesData.pas

import struct
from dataclasses import dataclass, asdict
from typing import List, BinaryIO, Tuple

# ---------- helpers ----------
def read_bytes(f: BinaryIO, n: int) -> bytes:
    b = f.read(n)
    if len(b) != n:
        raise EOFError("Arquivo terminou antes do esperado")
    return b

def cstr_from_fixed(buf: bytes) -> str:
    # ANSI zeroterm, corta no primeiro \x00 e strip
    return buf.split(b"\x00", 1)[0].decode("latin1", errors="ignore").strip()

def u8(f: BinaryIO) -> int:  return struct.unpack("<B", read_bytes(f,1))[0]
def u16(f: BinaryIO) -> int: return struct.unpack("<H", read_bytes(f,2))[0]
def u32(f: BinaryIO) -> int: return struct.unpack("<I", read_bytes(f,4))[0]
def i32(f: BinaryIO) -> int: return struct.unpack("<i", read_bytes(f,4))[0]

def many_u8(f: BinaryIO, n: int) -> Tuple[int, ...]:
    return struct.unpack("<" + "B"*n, read_bytes(f, n))
def many_u16(f: BinaryIO, n: int) -> Tuple[int, ...]:
    return struct.unpack("<" + "H"*n, read_bytes(f, n))
def many_i32(f: BinaryIO, n: int) -> Tuple[int, ...]:
    return struct.unpack("<" + "i"*n, read_bytes(f, 4*n))

# ---------- ItemList ----------
@dataclass
class ItemRecord:
    name: str
    name_en: str
    desc: str
    can_agroup: int
    item_type: int
    use_effect: int
    price_honor: int
    price_medal: int
    price_gold: int
    sell_price: int
    classe: int
    mesh_equip: int
    mesh_weapon: int
    icon_id: int
    set_id: int
    expires: int
    lvl_req: int
    duration: int
    atk_fis: int
    def_fis: int
    atk_mag: int
    def_mag: int
    hp: int
    mp: int
    rarity: int
    trade_type: int
    durability: int
    ef: Tuple[int,int,int]
    efv: Tuple[int,int,int]
    change: int
    reduction: int
    fortification: int
    rank: int
    max_lvl: int
    price_kind: int
    price_kind_val: int
    can_sealed: int

def read_item(f: BinaryIO) -> ItemRecord:
    name       = cstr_from_fixed(read_bytes(f, 64))
    name_en    = cstr_from_fixed(read_bytes(f, 64))
    desc       = cstr_from_fixed(read_bytes(f,128))

    can_agroup = u8(f)
    unk_bool   = u8(f)      # Unknow
    item_type  = u16(f)
    _ = read_bytes(f, 8)    # UnkValues[8]
    use_effect = u16(f)
    _ = read_bytes(f, 6)    # Unk_other[6]

    delay_use  = u32(f)
    price_honor= u32(f)
    price_medal= u32(f)
    price_gold = u32(f)
    sell_price = u32(f)
    _ = u32(f)              # Null_0

    classe     = u16(f)
    _ = u16(f)              # Null_00
    mesh_equip = u16(f)
    _ = read_bytes(f, 3)    # Null_1[3]
    _ = u16(f)              # Null_Mashid

    _ = u8(f)               # Null_0001 (Boolean)
    texture_id = u16(f)
    _ = read_bytes(f,4)     # Null_3[4]
    mesh_weapon= u16(f)

    icon_id    = u16(f)
    unk_1      = u16(f)
    set_id     = u16(f)
    _ = u16(f)              # Null_4
    expires    = u8(f)      # Boolean
    unk_bool2  = u8(f)      # Unk_Bool

    lvl_req    = u16(f)
    _ = u32(f)              # Null_001
    duration   = u32(f)
    _ = read_bytes(f,18)    # Nothing_2[18]

    atk_fis    = u16(f)
    def_fis    = u16(f)
    atk_mag    = u16(f)
    def_mag    = u16(f)

    _ = read_bytes(f,6)     # Null0[6]
    hp         = u16(f)
    mp         = u16(f)
    _ = read_bytes(f,14)    # Null001[14]
    rarity     = u8(f)      # TypeItem
    _ = u16(f)              # Null01
    trade_type = u8(f)      # TypeTrade
    _ = read_bytes(f,12)    # Null02[12]
    durability = u8(f)
    _ = read_bytes(f,5)     # Null_5[5]

    ef         = (u16(f), u16(f), u16(f))
    efv        = (u16(f), u16(f), u16(f))

    change     = u8(f)
    reduction  = u8(f)
    fortif     = u8(f)
    rank       = u8(f)

    _ = u32(f)              # Null
    max_lvl    = u32(f)
    _ = u32(f)              # Null3
    price_kind = u16(f)
    price_kind_val = u16(f)
    _ = read_bytes(f,16)    # Null4[16]
    can_sealed = u16(f)
    _ = u16(f)              # Null5

    return ItemRecord(
        name=name, name_en=name_en, desc=desc,
        can_agroup=can_agroup, item_type=item_type, use_effect=use_effect,
        price_honor=price_honor, price_medal=price_medal, price_gold=price_gold,
        sell_price=sell_price, classe=classe, mesh_equip=mesh_equip,
        mesh_weapon=mesh_weapon, icon_id=icon_id, set_id=set_id, expires=expires,
        lvl_req=lvl_req, duration=duration, atk_fis=atk_fis, def_fis=def_fis,
        atk_mag=atk_mag, def_mag=def_mag, hp=hp, mp=mp, rarity=rarity,
        trade_type=trade_type, durability=durability, ef=ef, efv=efv,
        change=change, reduction=reduction, fortification=fortif,
        rank=rank, max_lvl=max_lvl, price_kind=price_kind,
        price_kind_val=price_kind_val, can_sealed=can_sealed
    )

def read_itemlist(path: str, max_records: int = 26000) -> List[ItemRecord]:
    items: List[ItemRecord] = []
    with open(path, "rb") as f:
        for i in range(max_records):
            try:
                items.append(read_item(f))
            except EOFError:
                break
    return items

# ---------- SkillList ----------
@dataclass
class SkillRecord:
    index: int
    min_level: int
    level: int
    classification: int
    name_en: str
    name: str
    skill_points: int
    learn_costs: int
    classe: int
    mp: int
    pre_cooldown: int
    cooldown: int
    cooldown_type: int
    target_type: int
    unique_type: int
    max_targets: int
    range: int
    damage_range: int
    success_rate: int
    aggressive: int
    buff_debuff: int
    attribute: int
    damage: int
    adicional: int
    ef: Tuple[int,int,int,int]
    efv: Tuple[int,int,int,int]
    duration: int
    cast_time: int
    target_animation: int
    anim: int
    description: str

def read_skill(f: BinaryIO) -> SkillRecord:
    # Cabeçalho
    index        = u32(f)
    min_level    = u32(f)
    _            = u32(f)   # Null_3
    level        = u32(f)
    classification = u32(f)

    name_en      = cstr_from_fixed(read_bytes(f,64))
    name         = cstr_from_fixed(read_bytes(f,64))

    skill_points = u32(f)
    learn_costs  = u32(f)
    classe       = u32(f)

    _ = read_bytes(f,12)    # Unk_0[12]

    mp           = u32(f)
    unk1         = u32(f)   # Unk_1
    pre_cd       = u32(f)
    cd           = u32(f)
    cd_type      = u32(f)
    target_type  = u32(f)
    unique_type  = u32(f)
    max_targets  = u32(f)
    unknown1     = u32(f)
    sk_range     = u32(f)
    dmg_range    = u32(f)
    success_rate = u32(f)
    aggressive   = u32(f)
    buff_debuff  = u32(f)
    attribute    = i32(f)
    _ = read_bytes(f,16)    # Unk_2[16]
    damage       = u32(f)
    adicional    = u32(f)
    _ = u32(f)              # Unk_4

    ef           = many_i32(f,4)
    efv          = many_i32(f,4)

    duration     = u32(f)
    _ = read_bytes(f,24)    # Unk_5[24]
    cast_time    = u32(f)
    _ = read_bytes(f,24)    # Unk_6[24]
    target_anim  = u32(f)
    _ = u32(f)              # Unk_7
    anim         = u32(f)
    _ = read_bytes(f,68)    # Unk_8[68]
    description  = cstr_from_fixed(read_bytes(f,288))

    return SkillRecord(
        index=index, min_level=min_level, level=level, classification=classification,
        name_en=name_en, name=name, skill_points=skill_points, learn_costs=learn_costs,
        classe=classe, mp=mp, pre_cooldown=pre_cd, cooldown=cd, cooldown_type=cd_type,
        target_type=target_type, unique_type=unique_type, max_targets=max_targets,
        range=sk_range, damage_range=dmg_range, success_rate=success_rate,
        aggressive=aggressive, buff_debuff=buff_debuff, attribute=attribute,
        damage=damage, adicional=adicional, ef=ef, efv=efv, duration=duration,
        cast_time=cast_time, target_animation=target_anim, anim=anim,
        description=description
    )

def read_skilllist(path: str, max_records: int = 12000) -> List[SkillRecord]:
    skills: List[SkillRecord] = []
    with open(path, "rb") as f:
        for i in range(max_records):
            try:
                skills.append(read_skill(f))
            except EOFError:
                break
    return skills

# ---------- CLI de teste ----------
if __name__ == "__main__":
    # Ajuste os caminhos:
    ITEMLIST_PATH = "ItemList.bin"
    SKILLLIST_PATH = "SkillList.bin"

    try:
        items = read_itemlist(ITEMLIST_PATH)
        print(f"[ItemList] lidos: {len(items)}")
        for it in items[:5]:
            print(asdict(it))
    except Exception as e:
        print("[ItemList] erro:", e)

    try:
        skills = read_skilllist(SKILLLIST_PATH)
        print(f"[SkillList] lidos: {len(skills)}")
        for sk in skills[:5]:
            print(asdict(sk))
    except Exception as e:
        print("[SkillList] erro:", e)






