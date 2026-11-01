from flask import Flask, render_template, request, redirect, url_for, session, flash
import configparser
import os
import pymysql
import hashlib

app = Flask(__name__)
app.secret_key = os.environ.get("AIKA_WEB_SECRET", "asdkm32l;kjndfsjkn3425pnjk")

# Mesma conexao do servidor: secao [MySQL] de Bin\AikaServer.ini (fora do git).
# Variaveis AIKA_DB_* tem prioridade.
_ini = configparser.ConfigParser(interpolation=None, strict=False)
_ini.read(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "AikaServer.ini"),
          encoding="latin-1")
_mysql = _ini["MySQL"] if _ini.has_section("MySQL") else {}


def _db_setting(env, key, default):
    return os.environ.get(env) or str(_mysql.get(key, default)).strip()


# Database connection
def get_db():
    return pymysql.connect(
        host=_db_setting("AIKA_DB_HOST", "Server", "localhost"),
        port=int(_db_setting("AIKA_DB_PORT", "Port", "3306")),
        user=_db_setting("AIKA_DB_USER", "Username", "root"),
        password=_db_setting("AIKA_DB_PASSWORD", "Password", ""),
        database=_db_setting("AIKA_DB_NAME", "Database", "aika_db4"),
        cursorclass=pymysql.cursors.DictCursor
    )

@app.route("/")
def index():
    user = session.get("username")

    if user:
        # Usuário logado → carregar dados do painel lateral
        db = get_db()
        cur = db.cursor()
        cur.execute("""
            SELECT id, cash, nation, n.nation_name
            FROM accounts a
            LEFT JOIN nations n ON a.nation = n.nation_id
            WHERE username=%s
        """, (user,))
        conta = cur.fetchone()

        # Carregar personagens
        cur.execute("""
            SELECT name, classinfo, level, gold
            FROM characters
            WHERE owner_accid=%s
            ORDER BY slot ASC
        """, (conta["id"],))
        personagens = cur.fetchall()
    else:
        conta = None
        personagens = None

    return render_template("index.html", user=user, conta=conta, personagens=personagens)


# === REGISTRO ===
@app.route("/register", methods=["GET", "POST"])
def register():
    if request.method == "POST":
        username = request.form.get("username")
        email = request.form.get("email")
        password = request.form.get("password")

        if not username or not password or not email:
            flash("Preencha todos os campos.")
            return redirect(url_for("register"))

        password_md5 = hashlib.md5(password.encode()).hexdigest()

        db = get_db()
        cur = db.cursor()

        # ✅ 1. BUSCA O ÚLTIMO forum_id
        cur.execute("SELECT COALESCE(MAX(forum_id), 0) + 1 AS next_forum_id FROM accounts")
        next_forum_id = cur.fetchone()["next_forum_id"]

        try:
            # ✅ 2. USA O próximo forum_id no INSERT
            cur.execute("""
                INSERT INTO accounts (forum_id, username, password_hash, mail, time_created, ip_created)
                VALUES (%s, %s, %s, %s, UNIX_TIMESTAMP(), %s)
            """, (next_forum_id, username, password_md5, email, request.remote_addr))

            db.commit()
        except Exception as e:
            flash(f"Erro ao registrar: {e}")
            return redirect(url_for("register"))

        flash("Conta criada com sucesso! Faça login.")
        return redirect(url_for("login"))

    return render_template("register.html")


# === LOGIN ===
@app.route("/login", methods=["GET", "POST"])
def login():
    if request.method == "POST":
        username = request.form["username"]
        password = request.form["password"]

        db = get_db()
        cur = db.cursor()
        cur.execute("SELECT id, password_hash FROM accounts WHERE username=%s", (username,))
        result = cur.fetchone()

        if not result:
            flash("Usuário não encontrado.")
            return redirect(url_for("login"))

        user_id = result["id"]
        password_hash = result["password_hash"]

        if hashlib.md5(password.encode()).hexdigest() != password_hash:
            flash("Senha incorreta.")
            return redirect(url_for("login"))

        session["user_id"] = user_id
        session["username"] = username
        
        # ✅ Redireciona para a página inicial, não painel
        return redirect(url_for("index"))   

    return render_template("login.html")

# === PAINEL ===
@app.route("/painel")
def painel():
    if "username" not in session:
        return redirect(url_for("login"))
    
    db = get_db()
    cur = db.cursor()

    # Pegar dados da conta + Nome da nação
    cur.execute("""
        SELECT a.id, a.cash, a.nation, n.nation_name
        FROM accounts a
        LEFT JOIN nations n ON a.nation = n.nation_id
        WHERE a.username=%s
    """, (session["username"],))
    conta = cur.fetchone()

    if not conta:
        return redirect(url_for("logout"))

    # Se não houver nação, mostrar "Sem Nação"
    conta["nation_name"] = conta["nation_name"] if conta["nation_name"] else "Sem Nação"

    # Formatar CASH com separador de milhar (10.000)
    if conta["cash"] is not None:
        conta["cash"] = f"{conta['cash']:,}".replace(",", ".")

    # Pegar personagens da conta
    cur.execute("""
        SELECT name, classinfo, level, gold, guildindex
        FROM characters
        WHERE owner_accid=%s
        ORDER BY slot ASC
    """, (conta["id"],))
    personagens = cur.fetchall()

    # Mapeamento correto das classes
    class_names = {
        1: "Guerreiro",
        11: "Templária",
        21: "Atirador",
        31: "Pistoleira",
        41: "Feiticeiro",
        51: "Clérigo"
    }

    for p in personagens:
        # Nome da classe
        p["class_name"] = class_names.get(p["classinfo"], "Desconhecida")

        # Formatar GOLD com separador de milhar também
        if p["gold"] is not None:
            p["gold"] = f"{p['gold']:,}".replace(",", ".")


    return render_template("painel.html", username=session["username"], conta=conta, personagens=personagens)


# === LOGOUT ===
@app.route("/logout")
def logout():
    session.clear()
    return redirect("/")


if __name__ == "__main__":
    app.run(debug=False)
