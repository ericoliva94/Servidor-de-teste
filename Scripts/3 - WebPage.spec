# -*- mode: python ; coding: utf-8 -*-


a = Analysis(
    ['D:/Aika Lore Community/Server/Bin/WebPage/WebPage.py'],
    pathex=[],
    binaries=[],
    datas=[('D:/Aika Lore Community/Server/Bin/WebPage/templates', 'templates'), ('D:/Aika Lore Community/Server/Bin/WebPage/static', 'static')],
    hiddenimports=[],
    hookspath=[],
    hooksconfig={},
    runtime_hooks=[],
    excludes=[],
    noarchive=False,
    optimize=0,
)
pyz = PYZ(a.pure)

exe = EXE(
    pyz,
    a.scripts,
    a.binaries,
    a.datas,
    [],
    name='3 - WebPage',
    debug=False,
    bootloader_ignore_signals=False,
    strip=False,
    upx=True,
    upx_exclude=[],
    runtime_tmpdir=None,
    console=True,
    disable_windowed_traceback=False,
    argv_emulation=False,
    target_arch=None,
    codesign_identity=None,
    entitlements_file=None,
)
