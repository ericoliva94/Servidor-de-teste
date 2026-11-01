# -*- coding: utf-8 -*-
# AdminWeb com Flask + Socket.IO (tempo real)
import os, threading, time
from flask import Flask, render_template, send_from_directory, jsonify, request
from flask_socketio import SocketIO, emit
try:
    import psutil
except Exception:
    psutil = None

# ==== Integração com seus servidores (se existirem) ====
# Ajuste estes imports conforme seu projeto.
try:
    # LoginServer global (do seu LoginSocket.py traduzido)
    from Src.Connections.LoginSocket import LoginServer  # TLoginSocket global
except Exception:
    LoginServer = None

try:
    # Estrutura de servidores/channels do GameServer (se houver)
    from Src.Data.GlobalDefs import Servers  # lista/array de servers por canal
except Exception:
    Servers = None

BASE_DIR = os.path.dirname(os.path.abspath(__file__))

app = Flask(
    "AikaAdminWeb",
    template_folder=os.path.join(BASE_DIR, "templates"),
    static_folder=os.path.join(BASE_DIR, "static")
)
# cors_allowed_origins="*" para conseguir abrir no celular também
socketio = SocketIO(app, cors_allowed_origins="*", async_mode="eventlet")


# ==============================
# DADOS EM MEMÓRIA (TEMPORÁRIO)
# ==============================
SERVER_STATUS = "Offline"
MYSQL_STATUS = False
SERVER_TIME = "00:00:00"
PLAYERS = {
    "exion": [],
    "lenaria": [],
    "ostyrion": []
}
LOGS = []
CHAT = {
    "all": [],
    "exion": [],
    "lenaria": [],
    "ostyrion": []
}



# ========================
# ROTAS DO PAINEL
# ========================
@app.route("/")
def root():
    return render_template("dashboard.html")

@app.route("/dashboard")
def dashboard():
    return render_template("dashboard.html")

@app.route("/config")
def config():
    return render_template("config.html")

@app.route("/data/<path:filename>")
def data_files(filename):
    return send_from_directory(os.path.join(BASE_DIR, "data"), filename)

@app.route("/images/<path:filename>")
def image_files(filename):
    return send_from_directory(os.path.join(BASE_DIR, "images"), filename)

# ========================
# API (fallback/uso pontual)
# ========================
@app.route("/api/status")
def api_status():
    return jsonify({
        "server_status": SERVER_STATUS,
        "mysql": MYSQL_STATUS,
        "server_time": SERVER_TIME
    })

@app.route("/api/players")
def api_players():
    return jsonify(PLAYERS)

@app.route("/api/logs")
def api_logs():
    return jsonify({ "logs": LOGS })

@app.route("/api/chat")
def api_chat():
    nation = request.args.get("nation", "all")
    return jsonify({ "messages": CHAT.get(nation, []) })


@app.route("/api/console", methods=["POST"])
def api_console():
    data = request.json
    cmd = data.get("cmd", "")
    LOGS.append(f"[CMD] {cmd}")
    return jsonify({ "ok": True })


@app.route("/api/server/send", methods=["POST"])
def api_send():
    """Exemplo: enviar mensagem global pelo GameServer/Channel"""
    msg = (request.json or {}).get("message", "")
    channel = int((request.json or {}).get("channel", 1))
    ok = _send_server_message(msg, channel)
    return jsonify({"ok": ok})

@app.route("/api/server/control", methods=["POST"])
def api_server_control():
    data = request.json
    action = data.get("action", "")

    LOGS.append(f"[SERVER] ação recebida: {action}")

    # Aqui depois conectaremos com LoginServer / GameServer real
    return jsonify({ "ok": True })

# ========================
# SOCKET.IO (tempo real)
# ========================
# Namespace único '/admin' para o painel
NS = "/admin"

@socketio.on("connect", namespace=NS)
def on_connect():
    emit("hello", {"msg": "connected"}, namespace=NS)

@socketio.on("request_status", namespace=NS)
def on_request_status(_data=None):
    emit("status", _collect_status_snapshot(), namespace=NS)

@socketio.on("request_players", namespace=NS)
def on_request_players(data=None):
    channel = (data or {}).get("channel", 1)
    players = _collect_players(channel)
    emit("players", {"channel": channel, "players": players}, namespace=NS)

@socketio.on("command", namespace=NS)
def on_command(data):
    """
    data = {
      "type": "broadcast|kick|shutdown|restart|send_packet",
      "channel": 1,
      ... payload ...
    }
    """
    cmd = (data or {}).get("type")
    ch  = int((data or {}).get("channel", 1))
    ok, detail = _dispatch_command(cmd, ch, data or {})
    emit("command_result", {"ok": ok, "detail": detail}, namespace=NS)

# ========================
# HEARTBEAT: push periódico p/ o painel
# ========================
def _heartbeat():
    while True:
        try:
            socketio.emit("status", _collect_status_snapshot(), namespace=NS)
        except Exception:
            pass
        time.sleep(2)

def _collect_status_snapshot():
    # CPU/RAM
    cpu = psutil.cpu_percent() if psutil else 0.0
    ram = psutil.virtual_memory().percent if psutil else 0.0

    # LoginServer
    login_online = False
    if LoginServer is not None:
        try:
            login_online = bool(getattr(LoginServer, "IsActive", False))
        except Exception:
            login_online = False

    # GameServers por canal (se existir estrutura)
    channels = []
    if Servers is not None:
        try:
            for idx, srv in enumerate(Servers):
                if not srv:
                    continue
                chan = {
                    "channel": idx,
                    "is_active": bool(getattr(srv, "IsActive", False)),
                    "active_players": int(getattr(srv, "ActivePlayersNowHere", 0))
                }
                channels.append(chan)
        except Exception:
            pass

    return {
        "cpu": cpu, "ram": ram,
        "login_server": "Online" if login_online else "Offline",
        "channels": channels
    }

def _collect_players(channel=1):
    """
    Percorre Servers[channel].Players (array TPlayer) e coleta os ativos.
    Retorna uma lista simples para render no front.
    """
    out = []
    if Servers is None:
        return out
    try:
        srv = Servers[channel]
        players = getattr(srv, "Players", [])
        for p in players:
            try:
                if not getattr(p, "Base", None):  # estrutura do seu TPlayer
                    continue
                if not getattr(p.Base, "IsActive", False):
                    continue
                name = str(getattr(p.Base.Character, "Name", b"") or b"").strip().strip("\x00")
                cid  = int(getattr(p.Base, "ClientID", 0))
                lvl  = int(getattr(p.Base.Character, "Level", 0))
                nat  = int(getattr(p.Base.Character, "Nation", 0))
                out.append({"cid": cid, "name": name, "level": lvl, "nation": nat})
            except Exception:
                continue
    except Exception:
        pass
    return out

def _send_server_message(message, channel=1):
    """
    Equivale à SendServerMsg do seu ServerSocket.pas.
    Se houver método Python exposto, chame aqui.
    """
    if not message:
        return False
    if Servers is None:
        return False
    try:
        srv = Servers[channel]
        # se sua API Python tiver o método: srv.SendServerMsg(mensagem, ...)
        if hasattr(srv, "SendServerMsg"):
            srv.SendServerMsg(message)
            return True
    except Exception:
        pass
    return False

def _dispatch_command(cmd, channel, payload):
    try:
        if cmd == "broadcast":
            ok = _send_server_message(payload.get("message", ""), channel)
            return ok, "broadcast sent"
        if cmd == "kick":
            # exemplo: payload{"cid": 123}
            cid = int(payload.get("cid", 0))
            if Servers:
                srv = Servers[channel]
                if hasattr(srv, "Disconnect"):
                    srv.Disconnect(cid)
                    return True, f"kicked {cid}"
            return False, "kick failed"
        if cmd == "shutdown":
            if Servers:
                srv = Servers[channel]
                if hasattr(srv, "CloseServer"):
                    srv.CloseServer()
                    return True, "server closed"
            return False, "shutdown failed"
        if cmd == "restart":
            # simples: fecha e StartServer de novo
            if Servers:
                srv = Servers[channel]
                if hasattr(srv, "CloseServer") and hasattr(srv, "StartServer"):
                    srv.CloseServer()
                    time.sleep(0.5)
                    srv.StartServer()
                    return True, "server restarted"
            return False, "restart failed"
        if cmd == "send_packet":
            # payload: {"cid":..., "raw_hex":"AA BB CC ..."}
            cid = int(payload.get("cid", 0))
            raw = (payload.get("raw_hex", "") or "").replace(" ", "")
            data = bytes.fromhex(raw) if raw else b""
            if Servers:
                srv = Servers[channel]
                # se houver método para enviar pacote bruto:
                if hasattr(srv, "SendPacketTo"):
                    # opte por não cifrar aqui; seu método cuida disso
                    srv.SendPacketTo(cid, data, len(data), True)
                    return True, "packet sent"
            return False, "send_packet failed"
    except Exception as e:
        return False, f"error: {e}"
    return False, "unknown command"

# thread do heartbeat
threading.Thread(target=_heartbeat, daemon=True).start()

if __name__ == "__main__":
    print("[AdminWeb] Painel (Socket.IO) em http://127.0.0.1:5000")
    # Para Windows, eventlet é a escolha mais simples
    socketio.run(app, host="0.0.0.0", port=5000, debug=True)
