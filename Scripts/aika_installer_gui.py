"""Assistente local de instalacao do AIKA para Windows.

Coloque os instaladores e o dump em Scripts\\resources antes de executar.
Este programa nunca tenta configurar o roteador; ele apenas mostra as instrucoes.
"""

from __future__ import annotations

import ctypes
from datetime import datetime
import ipaddress
import json
import os
import re
import shutil
import socket
import subprocess
import sys
import threading
import webbrowser
from pathlib import Path
from tkinter import END, BOTH, LEFT, RIGHT, X, Y, BooleanVar, StringVar, Tk
from tkinter import filedialog, messagebox, ttk
from tkinter.scrolledtext import ScrolledText


CURRENT_LANGUAGE = "en"
PORTUGUESE_TO_ENGLISH = {
    "Instalacao local, banco, firewall e inicializacao. O roteador permanece manual.": "Local installation, database, firewall, and startup. Router setup remains manual.",
    "1. Componentes do pacote": "1. Package components",
    "Instalador MySQL": "MySQL installer",
    "Instalador Navicat": "Navicat installer",
    "Dump do banco AIKA": "AIKA database dump",
    "Selecionar": "Browse",
    "Instalar MySQL": "Install MySQL",
    "Instalar Navicat": "Install Navicat",
    "Atualizar estado": "Refresh status",
    "2. Banco de dados": "2. Database",
    "Banco": "Database",
    "Usuario": "User",
    "Senha SQL": "SQL password",
    "A senha fica somente na memoria desta sessao.": "The password stays only in this session's memory.",
    "Importar todos os dumps": "Import all dumps",
    "Exportar bancos + configs": "Export databases + configs",
    "Provisionar admin": "Provision admin",
    "Aplicar senha SQL em todas as configuracoes MySQL": "Apply SQL password to all MySQL configurations",
    "3. Firewall do Windows": "3. Windows Firewall",
    "Portas TCP": "TCP ports",
    "IP remoto": "Remote IP",
    "Permitir MySQL (3306) somente se for necessario": "Allow MySQL (3306) only when needed",
    "Criar regras privadas do Firewall": "Create private Firewall rules",
    "Execucao": "Runtime",
    "Inicia os executaveis existentes em Server\\Bin.": "Starts the existing executables in Server\\Bin.",
    "Iniciar Login + Game + Website": "Start Login + Game + Website",
    "Abrir website local": "Open local website",
    "Encaminhamento no roteador": "Router port forwarding",
    "Registro": "Log",
    "IP local deste computador:": "This computer's local IP:",
    "IP local nao identificado": "Local IP not detected",
    "1. Reserve esse IP no DHCP do roteador.": "1. Reserve this IP in the router DHCP settings.",
    "2. Entre no painel do roteador (normalmente 192.168.0.1 ou 192.168.1.1).": "2. Sign in to the router panel (usually 192.168.0.1 or 192.168.1.1).",
    "3. Procure NAT, Port Forwarding ou Servidor Virtual.": "3. Find NAT, Port Forwarding, or Virtual Server.",
    "4. Encaminhe TCP 8831, 8822 e 8090 para o IP local.": "4. Forward TCP 8831, 8822, and 8090 to the local IP.",
    "5. Encaminhe 3306 somente se acesso externo ao MySQL for indispensavel.": "5. Forward 3306 only when external MySQL access is essential.",
    "6. Teste de outra rede; se falhar, confirme CGNAT com a operadora.": "6. Test from another network; if it fails, confirm CGNAT with your ISP.",
    "Nao existe automacao segura e universal para roteadores de marcas diferentes.": "There is no safe universal automation for routers from different vendors.",
    "Arquivo ausente": "File missing",
    "Selecione o instalador do": "Select the installer for",
    "Confirmar instalacao": "Confirm installation",
    "Abrir o instalador do": "Open the installer for",
    "mysql.exe ausente": "mysql.exe missing",
    "Instale o MySQL ou selecione mysql.exe.": "Install MySQL or select mysql.exe.",
    "Porta invalida": "Invalid port",
    "Informe uma porta MySQL valida.": "Enter a valid MySQL port.",
    "Dump ausente": "Dump missing",
    "Selecione o dump .sql do AIKA.": "Select the AIKA .sql dump.",
    "Confirmar importacao": "Confirm import",
    "Importar": "Import",
    "Isso pode sobrescrever dados presentes no dump.": "This may overwrite data included in the dump.",
    "Importando dump. O MySQL pode solicitar a senha no console aberto.": "Importing dump.",
    "Dump importado com sucesso.": "Dump imported successfully.",
    "Importacao terminou com codigo": "Import finished with exit code",
    "Dumps ausentes": "Dumps missing",
    "Coloque todos os dumps .sql em Scripts\\resources.": "Place all .sql dumps in Scripts\\resources.",
    "Confirmar importacao completa": "Confirm complete import",
    "Importar todos os": "Import all",
    "dumps encontrados?": "dumps found?",
    "Cada dump deve conter CREATE DATABASE/USE quando necessario.": "Each dump must contain CREATE DATABASE/USE when needed.",
    "Importando:": "Importing:",
    "Falhou": "Failed",
    "Importado:": "Imported:",
    "Todos os dumps foram importados.": "All dumps were imported.",
    "Importacao terminou com": "Import finished with",
    "falha(s).": "failure(s).",
    "Template ausente": "Template missing",
    "Arquivo nao encontrado:": "File not found:",
    "Confirmar administrador": "Confirm administrator",
    "Criar ou atualizar a conta admin com senha MD5 de 'admin' e privilegio 5?": "Create or update the admin account with the MD5 password for 'admin' and privilege 5?",
    "A parte do personagem exige a adaptacao do template para o dump usado.": "The character section requires adapting the template to the selected dump.",
    "Provisionando conta admin pelo template SQL.": "Provisioning the admin account with the SQL template.",
    "Conta admin provisionada.": "Admin account provisioned.",
    "Provisionamento terminou com codigo": "Provisioning finished with exit code",
    "mysqldump ausente": "mysqldump missing",
    "Nao foi encontrado mysqldump.exe junto do mysql.exe.": "mysqldump.exe was not found next to mysql.exe.",
    "Bancos nao encontrados": "Databases not found",
    "Nenhum Database foi encontrado nas secoes [MySQL].": "No Database setting was found in [MySQL] sections.",
    "Confirmar backup completo": "Confirm full backup",
    "Exportar todos os bancos configurados": "Export all configured databases",
    "e todas as configuracoes em Scripts\\backups?": "and all configurations to Scripts\\backups?",
    "O backup conterá senhas e deve permanecer privado.": "The backup contains passwords and must remain private.",
    "Exportando banco:": "Exporting database:",
    "Exportado:": "Exported:",
    "Backup concluido em": "Backup completed at",
    "Backup criado com": "Backup created with",
    "Senha SQL ausente": "SQL password missing",
    "Informe a senha SQL antes de sincronizar as configuracoes.": "Enter the SQL password before synchronizing configurations.",
    "Configuracoes ausentes": "Configurations missing",
    "Nenhum arquivo de configuracao foi encontrado em Server.": "No configuration file was found in Server.",
    "Confirmar senha SQL": "Confirm SQL password",
    "Substituir Password e PasswordGM nas secoes [MySQL] de": "Replace Password and PasswordGM in [MySQL] sections of",
    "arquivos de configuracao?": "configuration files?",
    "Esta alteracao grava a senha em texto simples, como o servidor exige.": "This writes the password in plain text, as required by the server.",
    "Nao foi possivel ler": "Could not read",
    "Senha SQL sincronizada em": "SQL password synchronized in",
    "arquivo(s) de configuracao MySQL.": "MySQL configuration file(s).",
    "Portas invalidas": "Invalid ports",
    "Use portas TCP separadas por virgula.": "Use TCP ports separated by commas.",
    "IP invalido": "Invalid IP",
    "Use Any, um IP ou uma rede CIDR, por exemplo 203.0.113.10 ou 203.0.113.0/24.": "Use Any, an IP, or a CIDR network, for example 203.0.113.10 or 203.0.113.0/24.",
    "Confirmar firewall": "Confirm firewall",
    "Criar regras de entrada TCP no perfil Private do Windows?": "Create TCP inbound rules in the Windows Private profile?",
    "Sera solicitada permissao de administrador.": "Administrator permission will be requested.",
    "Regras do Firewall": "Firewall rules",
    "solicitacao enviada.": "request sent.",
    "o Windows recusou a elevacao": "Windows rejected elevation",
    "Executaveis ausentes": "Executables missing",
    "Nao encontrados:": "Not found:",
    "Iniciar servidor": "Start server",
    "Iniciar LoginServer, AikaServer e WebServer?": "Start LoginServer, AikaServer, and WebServer?",
    "Iniciado:": "Started:",
    "Abrindo:": "Launching:",
    "codigo de saida": "exit code",
    "Recursos:": "Resources:",
    "encontrados.": "found.",
    "nao encontrado": "not found",
    "encontrado": "found",
    "Falhou": "Failed",
}


def tr(text: str) -> str:
    if CURRENT_LANGUAGE == "pt":
        return text
    translated = text
    for portuguese, english in PORTUGUESE_TO_ENGLISH.items():
        translated = translated.replace(portuguese, english)
    return translated


def _localized_widget(widget_type):
    class LocalizedWidget(widget_type):
        def __init__(self, *args, **kwargs):
            if "text" in kwargs:
                kwargs["text"] = tr(kwargs["text"])
            super().__init__(*args, **kwargs)
    return LocalizedWidget


ttk.Label = _localized_widget(ttk.Label)
ttk.Button = _localized_widget(ttk.Button)
ttk.Checkbutton = _localized_widget(ttk.Checkbutton)
_showerror = messagebox.showerror
_askyesno = messagebox.askyesno
messagebox.showerror = lambda title, message, *args, **kwargs: _showerror(tr(title), tr(message), *args, **kwargs)
messagebox.askyesno = lambda title, message, *args, **kwargs: _askyesno(tr(title), tr(message), *args, **kwargs)


APP_DIR = Path(sys.executable).resolve().parent if getattr(sys, "frozen", False) else Path(__file__).resolve().parent
PROJECT_DIR = APP_DIR.parent
RESOURCES_DIR = APP_DIR / "resources"
SERVER_DIR = PROJECT_DIR / "Server" / "Bin"
SERVER_PROGRAMS = {
    "LoginServer.exe": SERVER_DIR / "LoginServer.exe",
    "AikaServer.exe": SERVER_DIR / "AikaServer.exe",
    "WebServer.exe": SERVER_DIR / "WebServer.exe",
}
DEFAULT_PORTS = "3306, 8090, 8822, 8831"
ADMIN_SQL = RESOURCES_DIR / "bootstrap_admin.sql"


def local_ip() -> str:
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        sock.connect(("8.8.8.8", 80))
        return sock.getsockname()[0]
    except OSError:
        return "IP local nao identificado"
    finally:
        sock.close()


def find_resource(stems: tuple[str, ...], suffixes: tuple[str, ...]) -> Path | None:
    if not RESOURCES_DIR.exists():
        return None
    for item in RESOURCES_DIR.iterdir():
        if item.is_file() and item.suffix.lower() in suffixes:
            name = item.name.lower()
            if any(stem in name for stem in stems):
                return item
    return None


class InstallerApp:
    def __init__(self, root: Tk) -> None:
        self.root = root
        self.root.title("AIKA Setup Station")
        self.root.geometry("1030x710")
        self.root.minsize(900, 630)
        self.root.configure(bg="#111722")

        self.mysql_installer = StringVar(value=self._initial_path(("mysql",), (".msi", ".exe")))
        self.navicat_installer = StringVar(value=self._initial_path(("navicat",), (".msi", ".exe")))
        self.database_dump = StringVar(value=self._initial_path(("aika", "database", "dump"), (".sql",)))
        self.mysql_bin = StringVar(value=self._detect_mysql_bin())
        self.db_host = StringVar(value="127.0.0.1")
        self.db_port = StringVar(value="3306")
        self.db_name = StringVar(value="aika_db4")
        self.db_user = StringVar(value="root")
        self.sql_password = StringVar(value="")
        self.language = StringVar(value="English" if CURRENT_LANGUAGE == "en" else "Português")
        self.allowed_ip = StringVar(value="Any")
        self.ports = StringVar(value=DEFAULT_PORTS)
        self.open_mysql = BooleanVar(value=False)

        self._style()
        self._build_ui()
        self.refresh_status()

    def _initial_path(self, stems: tuple[str, ...], suffixes: tuple[str, ...]) -> str:
        resource = find_resource(stems, suffixes)
        return str(resource) if resource else ""

    @staticmethod
    def _detect_mysql_bin() -> str:
        found = shutil.which("mysql.exe") or shutil.which("mysql")
        return found or ""

    def _style(self) -> None:
        style = ttk.Style()
        style.theme_use("clam")
        style.configure("TFrame", background="#111722")
        style.configure("Card.TFrame", background="#1a2333")
        style.configure("TLabel", background="#111722", foreground="#dce6f3", font=("Segoe UI", 10))
        style.configure("Card.TLabel", background="#1a2333", foreground="#dce6f3", font=("Segoe UI", 10))
        style.configure("Title.TLabel", background="#111722", foreground="#f4c66b", font=("Bahnschrift", 24, "bold"))
        style.configure("Subtitle.TLabel", background="#111722", foreground="#9eafc3", font=("Segoe UI", 10))
        style.configure("Section.TLabel", background="#1a2333", foreground="#f4c66b", font=("Bahnschrift", 13, "bold"))
        style.configure("TEntry", fieldbackground="#0d131e", foreground="#e8f0fa", insertcolor="#e8f0fa")
        style.configure("TButton", background="#2b6d73", foreground="#ffffff", borderwidth=0, padding=(10, 7), font=("Segoe UI", 9, "bold"))
        style.map("TButton", background=[("active", "#3e8b8f")])
        style.configure("Accent.TButton", background="#c47d2d")
        style.map("Accent.TButton", background=[("active", "#e0963e")])
        style.configure("TCheckbutton", background="#1a2333", foreground="#dce6f3")

    def _build_ui(self) -> None:
        header = ttk.Frame(self.root)
        header.pack(fill=X, padx=24, pady=(20, 10))
        ttk.Label(header, text="AIKA Setup Station", style="Title.TLabel").pack(anchor="w")
        language_box = ttk.Frame(header)
        language_box.pack(side=RIGHT, anchor="ne")
        ttk.Label(language_box, text="Language").pack(side=LEFT, padx=(0, 7))
        selector = ttk.Combobox(language_box, textvariable=self.language, values=("English", "Português"), state="readonly", width=11)
        selector.pack(side=RIGHT)
        selector.bind("<<ComboboxSelected>>", self.change_language)
        ttk.Label(
            header,
            text="Instalacao local, banco, firewall e inicializacao. O roteador permanece manual.",
            style="Subtitle.TLabel",
        ).pack(anchor="w", pady=(2, 0))

        body = ttk.Frame(self.root)
        body.pack(fill=BOTH, expand=True, padx=24, pady=(0, 20))
        body.columnconfigure(0, weight=3)
        body.columnconfigure(1, weight=2)
        body.rowconfigure(0, weight=1)

        left = ttk.Frame(body, style="Card.TFrame", padding=18)
        left.grid(row=0, column=0, sticky="nsew", padx=(0, 10))
        right = ttk.Frame(body, style="Card.TFrame", padding=18)
        right.grid(row=0, column=1, sticky="nsew")
        self._build_left(left)
        self._build_right(right)

    def _build_left(self, parent: ttk.Frame) -> None:
        ttk.Label(parent, text="1. Componentes do pacote", style="Section.TLabel").pack(anchor="w")
        self._file_row(parent, "Instalador MySQL", self.mysql_installer, (("Instaladores", "*.msi *.exe"),))
        self._file_row(parent, "Instalador Navicat", self.navicat_installer, (("Instaladores", "*.msi *.exe"),))
        self._file_row(parent, "Dump do banco AIKA", self.database_dump, (("SQL", "*.sql"),))
        self._file_row(parent, "mysql.exe", self.mysql_bin, (("Executavel", "mysql.exe"), ("Todos", "*.*")))

        actions = ttk.Frame(parent, style="Card.TFrame")
        actions.pack(fill=X, pady=(12, 16))
        ttk.Button(actions, text="Instalar MySQL", command=lambda: self.install_component(self.mysql_installer.get(), "MySQL")).pack(side=LEFT, padx=(0, 8))
        ttk.Button(actions, text="Instalar Navicat", command=lambda: self.install_component(self.navicat_installer.get(), "Navicat")).pack(side=LEFT, padx=(0, 8))
        ttk.Button(actions, text="Atualizar estado", command=self.refresh_status).pack(side=LEFT)

        ttk.Separator(parent).pack(fill=X, pady=(0, 16))
        ttk.Label(parent, text="2. Banco de dados", style="Section.TLabel").pack(anchor="w")
        database_form = ttk.Frame(parent, style="Card.TFrame")
        database_form.pack(fill=X, pady=(8, 8))
        self._entry_row(database_form, "Host", self.db_host, 0, 0)
        self._entry_row(database_form, "Porta", self.db_port, 0, 2)
        self._entry_row(database_form, "Banco", self.db_name, 1, 0)
        self._entry_row(database_form, "Usuario", self.db_user, 1, 2)
        ttk.Label(database_form, text="Senha SQL", style="Card.TLabel").grid(row=2, column=0, sticky="w", padx=(0, 6), pady=3)
        ttk.Entry(database_form, textvariable=self.sql_password, show="*", width=18).grid(row=2, column=1, sticky="ew", padx=(0, 12), pady=3)
        ttk.Label(database_form, text="A senha fica somente na memoria desta sessao.", style="Card.TLabel").grid(row=3, column=0, columnspan=4, sticky="w", pady=(7, 0))
        db_actions = ttk.Frame(parent, style="Card.TFrame")
        db_actions.pack(fill=X, pady=(3, 16))
        ttk.Button(db_actions, text="Importar todos os dumps", command=self.import_all_dumps).pack(side=LEFT, padx=(0, 8))
        ttk.Button(db_actions, text="Exportar bancos + configs", command=self.export_all).pack(side=LEFT, padx=(0, 8))
        ttk.Button(db_actions, text="Provisionar admin", style="Accent.TButton", command=self.provision_admin).pack(side=LEFT)
        ttk.Button(parent, text="Aplicar senha SQL em todas as configuracoes MySQL", command=self.apply_sql_password).pack(anchor="w", pady=(0, 16))

        ttk.Separator(parent).pack(fill=X, pady=(0, 16))
        ttk.Label(parent, text="3. Firewall do Windows", style="Section.TLabel").pack(anchor="w")
        firewall = ttk.Frame(parent, style="Card.TFrame")
        firewall.pack(fill=X, pady=(8, 5))
        ttk.Label(firewall, text="Portas TCP", style="Card.TLabel").grid(row=0, column=0, sticky="w")
        ttk.Entry(firewall, textvariable=self.ports, width=27).grid(row=0, column=1, sticky="ew", padx=(8, 16))
        ttk.Label(firewall, text="IP remoto", style="Card.TLabel").grid(row=0, column=2, sticky="w")
        ttk.Entry(firewall, textvariable=self.allowed_ip, width=18).grid(row=0, column=3, sticky="ew", padx=(8, 0))
        firewall.columnconfigure(1, weight=1)
        ttk.Checkbutton(firewall, text="Permitir MySQL (3306) somente se for necessario", variable=self.open_mysql, style="TCheckbutton").grid(row=1, column=0, columnspan=4, sticky="w", pady=(8, 0))
        ttk.Button(parent, text="Criar regras privadas do Firewall", command=self.configure_firewall).pack(anchor="w", pady=(5, 0))

    def _build_right(self, parent: ttk.Frame) -> None:
        ttk.Label(parent, text="Execucao", style="Section.TLabel").pack(anchor="w")
        ttk.Label(parent, text="Inicia os executaveis existentes em Server\\Bin.", style="Card.TLabel").pack(anchor="w", pady=(6, 10))
        ttk.Button(parent, text="Iniciar Login + Game + Website", style="Accent.TButton", command=self.start_server).pack(fill=X)
        ttk.Button(parent, text="Abrir website local", command=lambda: webbrowser.open("http://127.0.0.1:8090")).pack(fill=X, pady=(8, 0))

        ttk.Separator(parent).pack(fill=X, pady=18)
        ttk.Label(parent, text="Encaminhamento no roteador", style="Section.TLabel").pack(anchor="w")
        tips = (
            f"IP local deste computador: {local_ip()}\n\n"
            "1. Reserve esse IP no DHCP do roteador.\n"
            "2. Entre no painel do roteador (normalmente 192.168.0.1 ou 192.168.1.1).\n"
            "3. Procure NAT, Port Forwarding ou Servidor Virtual.\n"
            "4. Encaminhe TCP 8831, 8822 e 8090 para o IP local.\n"
            "5. Encaminhe 3306 somente se acesso externo ao MySQL for indispensavel.\n"
            "6. Teste de outra rede; se falhar, confirme CGNAT com a operadora.\n\n"
            "Nao existe automacao segura e universal para roteadores de marcas diferentes."
        )
        tip_box = ScrolledText(parent, height=15, wrap="word", bg="#0d131e", fg="#dce6f3", insertbackground="#dce6f3", relief="flat", font=("Segoe UI", 10))
        tip_box.pack(fill=X, pady=(8, 0))
        tip_box.insert("1.0", tr(tips))
        tip_box.configure(state="disabled")

        ttk.Separator(parent).pack(fill=X, pady=18)
        ttk.Label(parent, text="Registro", style="Section.TLabel").pack(anchor="w")
        self.log_box = ScrolledText(parent, height=8, wrap="word", bg="#0d131e", fg="#b7d6c0", insertbackground="#dce6f3", relief="flat", font=("Cascadia Mono", 9))
        self.log_box.pack(fill=BOTH, expand=True, pady=(8, 0))

    def _file_row(self, parent: ttk.Frame, label: str, variable: StringVar, filetypes: tuple[tuple[str, str], ...]) -> None:
        row = ttk.Frame(parent, style="Card.TFrame")
        row.pack(fill=X, pady=(8, 0))
        ttk.Label(row, text=label, style="Card.TLabel", width=18).pack(side=LEFT)
        ttk.Entry(row, textvariable=variable).pack(side=LEFT, fill=X, expand=True, padx=(0, 7))
        ttk.Button(row, text="Selecionar", command=lambda: self.select_file(variable, filetypes)).pack(side=RIGHT)

    def _entry_row(self, parent: ttk.Frame, label: str, variable: StringVar, row: int, column: int) -> None:
        ttk.Label(parent, text=label, style="Card.TLabel").grid(row=row, column=column, sticky="w", padx=(0, 6), pady=3)
        ttk.Entry(parent, textvariable=variable, width=18).grid(row=row, column=column + 1, sticky="ew", padx=(0, 12), pady=3)

    def select_file(self, variable: StringVar, filetypes: tuple[tuple[str, str], ...]) -> None:
        selected = filedialog.askopenfilename(title=tr("Selecionar arquivo"), filetypes=filetypes)
        if selected:
            variable.set(selected)
            self.refresh_status()

    def log(self, text: str) -> None:
        self.log_box.insert(END, tr(text.rstrip()) + "\n")
        self.log_box.see(END)

    def change_language(self, _event=None) -> None:
        global CURRENT_LANGUAGE
        selected = "pt" if self.language.get() == "Português" else "en"
        if selected == CURRENT_LANGUAGE:
            return
        CURRENT_LANGUAGE = selected
        for child in self.root.winfo_children():
            child.destroy()
        self.root.title(tr("AIKA Setup Station"))
        self._build_ui()
        self.refresh_status()

    def refresh_status(self) -> None:
        resources = [self.mysql_installer.get(), self.navicat_installer.get(), self.database_dump.get()]
        found = sum(bool(path and Path(path).is_file()) for path in resources)
        mysql_status = "encontrado" if self.mysql_bin.get() and Path(self.mysql_bin.get()).is_file() else "nao encontrado"
        self.log(f"Recursos: {found}/3 encontrados. mysql.exe: {mysql_status}.")

    def install_component(self, raw_path: str, name: str) -> None:
        path = Path(raw_path)
        if not path.is_file():
            messagebox.showerror("Arquivo ausente", f"Selecione o instalador do {name}.")
            return
        if not messagebox.askyesno("Confirmar instalacao", f"Abrir o instalador do {name}?\n\n{path.name}"):
            return
        args = ["msiexec.exe", "/i", str(path)] if path.suffix.lower() == ".msi" else [str(path)]
        self.run_background(args, f"Instalador {name}")

    def mysql_command(self, database: str | None = None) -> list[str] | None:
        executable = Path(self.mysql_bin.get())
        if not executable.is_file():
            messagebox.showerror("mysql.exe ausente", "Instale o MySQL ou selecione mysql.exe.")
            return None
        try:
            port = int(self.db_port.get())
            if not 1 <= port <= 65535:
                raise ValueError
        except ValueError:
            messagebox.showerror("Porta invalida", "Informe uma porta MySQL valida.")
            return None
        command = [str(executable), "-h", self.db_host.get().strip(), "-P", str(port), "-u", self.db_user.get().strip()]
        if database:
            command.append(database)
        return command

    def sql_environment(self) -> dict[str, str]:
        environment = os.environ.copy()
        environment["MYSQL_PWD"] = self.sql_password.get()
        return environment

    def import_dump(self) -> None:
        dump = Path(self.database_dump.get())
        command = self.mysql_command(self.db_name.get().strip())
        if command is None:
            return
        if not dump.is_file():
            messagebox.showerror("Dump ausente", "Selecione o dump .sql do AIKA.")
            return
        if not messagebox.askyesno("Confirmar importacao", f"Importar {dump.name} no banco {self.db_name.get()}?\n\nIsso pode sobrescrever dados presentes no dump."):
            return

        def task() -> None:
            self.log("Importando dump. O MySQL pode solicitar a senha no console aberto.")
            with dump.open("rb") as source:
                result = subprocess.run(command, stdin=source, cwd=APP_DIR, env=self.sql_environment(), check=False)
            self.log("Dump importado com sucesso." if result.returncode == 0 else f"Importacao terminou com codigo {result.returncode}.")

        self.thread(task)

    def sql_dumps(self) -> list[Path]:
        if not RESOURCES_DIR.is_dir():
            return []
        return sorted(path for path in RESOURCES_DIR.rglob("*.sql") if path.resolve() != ADMIN_SQL.resolve())

    def import_all_dumps(self) -> None:
        dumps = self.sql_dumps()
        command = self.mysql_command()
        if command is None:
            return
        if not dumps:
            messagebox.showerror("Dumps ausentes", "Coloque todos os dumps .sql em Scripts\\resources.")
            return
        names = "\n".join(f"- {dump.relative_to(RESOURCES_DIR)}" for dump in dumps)
        if not messagebox.askyesno("Confirmar importacao completa", f"Importar todos os {len(dumps)} dumps encontrados?\n\n{names}\n\nCada dump deve conter CREATE DATABASE/USE quando necessario."):
            return

        def task() -> None:
            failures = 0
            for dump in dumps:
                self.log(f"Importando: {dump.name}")
                with dump.open("rb") as source:
                    result = subprocess.run(command, stdin=source, cwd=APP_DIR, env=self.sql_environment(), check=False)
                if result.returncode:
                    failures += 1
                    self.log(f"Falhou ({result.returncode}): {dump.name}")
                else:
                    self.log(f"Importado: {dump.name}")
            self.log("Todos os dumps foram importados." if not failures else f"Importacao terminou com {failures} falha(s).")

        self.thread(task)

    def provision_admin(self) -> None:
        command = self.mysql_command(self.db_name.get().strip())
        if command is None:
            return
        if not ADMIN_SQL.is_file():
            messagebox.showerror("Template ausente", f"Arquivo nao encontrado: {ADMIN_SQL}")
            return
        if not messagebox.askyesno(
            "Confirmar administrador",
            "Criar ou atualizar a conta admin com senha MD5 de 'admin' e privilegio 5?\n\n"
            "A parte do personagem exige a adaptacao do template para o dump usado.",
        ):
            return

        def task() -> None:
            self.log("Provisionando conta admin pelo template SQL.")
            with ADMIN_SQL.open("rb") as source:
                result = subprocess.run(command, stdin=source, cwd=APP_DIR, env=self.sql_environment(), check=False)
            self.log("Conta admin provisionada." if result.returncode == 0 else f"Provisionamento terminou com codigo {result.returncode}.")

        self.thread(task)

    @staticmethod
    def config_files() -> list[Path]:
        if not SERVER_DIR.parent.is_dir():
            return []
        files: list[Path] = []
        for suffix in ("*.ini", "*.cfg", "*.conf"):
            files.extend(SERVER_DIR.parent.rglob(suffix))
        return sorted(set(files))

    def configured_databases(self) -> list[str]:
        databases: set[str] = set()
        for config in self.config_files():
            try:
                lines = config.read_text(encoding="utf-8", errors="ignore").splitlines()
            except OSError:
                continue
            section = ""
            for line in lines:
                header = re.match(r"^\s*\[([^]]+)]", line)
                if header:
                    section = header.group(1).strip().lower()
                    continue
                match = re.match(r"^\s*database\s*=\s*(.+?)\s*$", line, flags=re.IGNORECASE)
                if match and section == "mysql" and match.group(1).strip():
                    databases.add(match.group(1).strip())
        if self.db_name.get().strip():
            databases.add(self.db_name.get().strip())
        return sorted(databases)

    def mysqldump_command(self, database: str) -> list[str] | None:
        mysql = Path(self.mysql_bin.get())
        candidates = [mysql.with_name("mysqldump.exe"), mysql.with_name("mysqldump")]
        found = next((path for path in candidates if path.is_file()), None)
        if found is None:
            executable = shutil.which("mysqldump.exe") or shutil.which("mysqldump")
            found = Path(executable) if executable else None
        if found is None:
            messagebox.showerror("mysqldump ausente", "Nao foi encontrado mysqldump.exe junto do mysql.exe.")
            return None
        try:
            port = int(self.db_port.get())
            if not 1 <= port <= 65535:
                raise ValueError
        except ValueError:
            messagebox.showerror("Porta invalida", "Informe uma porta MySQL valida.")
            return None
        return [str(found), "-h", self.db_host.get().strip(), "-P", str(port), "-u", self.db_user.get().strip(), "--routines", "--events", "--triggers", "--single-transaction", "--databases", database]

    def export_all(self) -> None:
        databases = self.configured_databases()
        if not databases:
            messagebox.showerror("Bancos nao encontrados", "Nenhum Database foi encontrado nas secoes [MySQL].")
            return
        if self.mysqldump_command(databases[0]) is None:
            return
        joined = ", ".join(databases)
        if not messagebox.askyesno("Confirmar backup completo", f"Exportar todos os bancos configurados ({joined}) e todas as configuracoes em Scripts\\backups?\n\nO backup conterá senhas e deve permanecer privado."):
            return

        def task() -> None:
            output_dir = APP_DIR / "backups" / datetime.now().strftime("%Y%m%d_%H%M%S")
            config_dir = output_dir / "configs"
            output_dir.mkdir(parents=True, exist_ok=False)
            config_dir.mkdir()
            failures = 0
            for database in databases:
                command = self.mysqldump_command(database)
                if command is None:
                    failures += 1
                    continue
                target = output_dir / f"{database}.sql"
                self.log(f"Exportando banco: {database}")
                with target.open("wb") as destination:
                    result = subprocess.run(command, stdout=destination, cwd=APP_DIR, env=self.sql_environment(), check=False)
                if result.returncode:
                    failures += 1
                    self.log(f"Falhou ({result.returncode}): {database}")
                else:
                    self.log(f"Exportado: {target.name}")
            for config in self.config_files():
                relative = config.relative_to(PROJECT_DIR)
                target = config_dir / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(config, target)
            self.log(f"Backup concluido em {output_dir}" if not failures else f"Backup criado com {failures} falha(s): {output_dir}")

        self.thread(task)

    def apply_sql_password(self) -> None:
        password = self.sql_password.get()
        if not password:
            messagebox.showerror("Senha SQL ausente", "Informe a senha SQL antes de sincronizar as configuracoes.")
            return
        configs = self.config_files()
        if not configs:
            messagebox.showerror("Configuracoes ausentes", "Nenhum arquivo de configuracao foi encontrado em Server.")
            return
        if not messagebox.askyesno("Confirmar senha SQL", f"Substituir Password e PasswordGM nas secoes [MySQL] de {len(configs)} arquivos de configuracao?\n\nEsta alteracao grava a senha em texto simples, como o servidor exige."):
            return
        changed_files = 0
        for config in configs:
            try:
                original = config.read_text(encoding="utf-8", errors="ignore")
            except OSError as error:
                self.log(f"Nao foi possivel ler {config}: {error}")
                continue
            section = ""
            changed = False
            output: list[str] = []
            for line in original.splitlines(keepends=True):
                header = re.match(r"^\s*\[([^]]+)]", line)
                if header:
                    section = header.group(1).strip().lower()
                if section == "mysql" and re.match(r"^\s*(password|passwordgm)\s*=", line, flags=re.IGNORECASE):
                    ending = "\r\n" if line.endswith("\r\n") else "\n" if line.endswith("\n") else ""
                    key = line.split("=", 1)[0].rstrip()
                    line = f"{key}={password}{ending}"
                    changed = True
                output.append(line)
            if changed:
                config.write_text("".join(output), encoding="utf-8")
                changed_files += 1
        self.log(f"Senha SQL sincronizada em {changed_files} arquivo(s) de configuracao MySQL.")
        self.refresh_status()

    def configure_firewall(self) -> None:
        try:
            ports = sorted({int(value.strip()) for value in self.ports.get().split(",") if value.strip()})
            if self.open_mysql.get():
                ports = sorted(set(ports + [3306]))
            if not ports or any(port < 1 or port > 65535 for port in ports):
                raise ValueError
        except ValueError:
            messagebox.showerror("Portas invalidas", "Use portas TCP separadas por virgula.")
            return
        remote_ip = self.allowed_ip.get().strip()
        if remote_ip.lower() != "any":
            try:
                ipaddress.ip_network(remote_ip, strict=False)
            except ValueError:
                messagebox.showerror("IP invalido", "Use Any, um IP ou uma rede CIDR, por exemplo 203.0.113.10 ou 203.0.113.0/24.")
                return
        if not messagebox.askyesno("Confirmar firewall", "Criar regras de entrada TCP no perfil Private do Windows?\nSera solicitada permissao de administrador."):
            return
        remote = "Any" if remote_ip.lower() == "any" else remote_ip
        port_list = ",".join(map(str, ports))
        command = (
            "$name='AIKA Server TCP'; "
            "Get-NetFirewallRule -DisplayName $name -ErrorAction SilentlyContinue | Remove-NetFirewallRule; "
            f"New-NetFirewallRule -DisplayName $name -Direction Inbound -Action Allow -Protocol TCP -LocalPort '{port_list}' -RemoteAddress '{remote}' -Profile Private"
        )
        self.run_elevated_powershell(command, "Regras do Firewall")

    def start_server(self) -> None:
        missing = [name for name, path in SERVER_PROGRAMS.items() if not path.is_file()]
        if missing:
            messagebox.showerror("Executaveis ausentes", "Nao encontrados: " + ", ".join(missing))
            return
        if not messagebox.askyesno("Iniciar servidor", "Iniciar LoginServer, AikaServer e WebServer?"):
            return
        for name, path in SERVER_PROGRAMS.items():
            subprocess.Popen([str(path)], cwd=SERVER_DIR)
            self.log(f"Iniciado: {name}")

    def run_background(self, command: list[str], label: str) -> None:
        def task() -> None:
            self.log(f"Abrindo: {label}")
            result = subprocess.run(command, cwd=APP_DIR, check=False)
            self.log(f"{label}: codigo de saida {result.returncode}.")

        self.thread(task)

    def run_elevated_powershell(self, command: str, label: str) -> None:
        def task() -> None:
            parameters = f'-NoProfile -ExecutionPolicy Bypass -Command "{command}"'
            result = ctypes.windll.shell32.ShellExecuteW(None, "runas", "powershell.exe", parameters, str(APP_DIR), 1)
            self.log(f"{label}: solicitacao enviada." if result > 32 else f"{label}: o Windows recusou a elevacao ({result}).")

        self.thread(task)

    def thread(self, target) -> None:
        threading.Thread(target=target, daemon=True).start()


def main() -> None:
    RESOURCES_DIR.mkdir(exist_ok=True)
    root = Tk()
    InstallerApp(root)
    root.mainloop()


if __name__ == "__main__":
    main()
