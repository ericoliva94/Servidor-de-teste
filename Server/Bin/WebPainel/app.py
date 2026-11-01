from flask import Flask, render_template, jsonify, request
from datetime import datetime

app = Flask(__name__)

# Sample data (in production, this would come from the database)
players_online = [
    {"id": 1, "name": "Warrior123", "level": 85, "class": "Warrior", "ip": "192.168.1.1", "status": "Online"},
    {"id": 2, "name": "MageKing", "level": 72, "class": "Mage", "ip": "192.168.1.2", "status": "Online"},
    {"id": 3, "name": "RogueNinja", "level": 90, "class": "Rogue", "ip": "192.168.1.3", "status": "Online"},
    {"id": 4, "name": "PaladinHero", "level": 78, "class": "Paladin", "ip": "192.168.1.4", "status": "Online"},
]

console_logs = [
    {"time": "10:32:15", "type": "info", "message": "Server started successfully"},
    {"time": "10:32:18", "type": "success", "message": "Database connection established"},
    {"time": "10:35:22", "type": "warning", "message": "High memory usage detected"},
]

@app.route('/')
def index():
    return render_template('painel.html')

@app.route('/api/players')
def get_players():
    return jsonify(players_online)

@app.route('/api/console/logs')
def get_console_logs():
    return jsonify(console_logs)

@app.route('/api/console/command', methods=['POST'])
def execute_command():
    data = request.json
    command = data.get('command', '')
    
    # Simulated command execution
    response = {
        "time": datetime.now().strftime("%H:%M:%S"),
        "type": "command",
        "message": f"> {command}",
        "response": f"Command '{command}' executed successfully"
    }
    
    console_logs.append(response)
    return jsonify(response)

@app.route('/api/player/action', methods=['POST'])
def player_action():
    data = request.json
    action = data.get('action')
    player_id = data.get('player_id')
    
    # Simulated player action
    return jsonify({
        "success": True,
        "message": f"Action '{action}' executed on player {player_id}"
    })

if __name__ == '__main__':
    app.run(debug=False, host='0.0.0.0', port=5001)
