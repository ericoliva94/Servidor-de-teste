// Navigation
const navItems = document.querySelectorAll('.nav-item');
const contentSections = document.querySelectorAll('.content-section');

navItems.forEach(item => {
    item.addEventListener('click', () => {
        const section = item.dataset.section;
        
        // Update the active navigation item
        navItems.forEach(nav => nav.classList.remove('active'));
        item.classList.add('active');
        
        // Update the active section
        contentSections.forEach(content => content.classList.remove('active'));
        document.getElementById(section).classList.add('active');
    });
});

// Console
const consoleOutput = document.getElementById('console-output');
const consoleInput = document.getElementById('console-input');
const consoleSend = document.getElementById('console-send');

function addConsoleLog(time, type, message) {
    const line = document.createElement('div');
    line.className = `console-line ${type}`;
    line.innerHTML = `
        <span class="console-time">${time}</span>
        <span class="console-message">${message}</span>
    `;
    consoleOutput.appendChild(line);
    consoleOutput.scrollTop = consoleOutput.scrollHeight;
}

function executeCommand(command) {
    if (!command.trim()) return;
    
    const time = new Date().toLocaleTimeString('en-US', { hour12: false });
    addConsoleLog(time, 'command', `> ${command}`);
    
    // Send the command to the backend
    fetch('/api/console/command', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify({ command })
    })
    .then(response => response.json())
    .then(data => {
        addConsoleLog(data.time, 'success', data.response);
    })
    .catch(error => {
        addConsoleLog(time, 'error', `Error: ${error.message}`);
    });
    
    consoleInput.value = '';
}

consoleSend.addEventListener('click', () => {
    executeCommand(consoleInput.value);
});

consoleInput.addEventListener('keypress', (e) => {
    if (e.key === 'Enter') {
        executeCommand(consoleInput.value);
    }
});

// Load console logs
fetch('/api/console/logs')
    .then(response => response.json())
    .then(logs => {
        logs.forEach(log => {
            addConsoleLog(log.time, log.type, log.message);
        });
    });

// Players
const playersTbody = document.getElementById('players-tbody');
const playerSearch = document.getElementById('player-search');
const playersCount = document.getElementById('players-count');
let allPlayers = [];

function renderPlayers(players) {
    playersTbody.innerHTML = '';
    
    players.forEach(player => {
        const row = document.createElement('tr');
        row.innerHTML = `
            <td>${player.id}</td>
            <td><strong>${player.name}</strong></td>
            <td>${player.level}</td>
            <td>${player.class}</td>
            <td>${player.ip}</td>
            <td>
                <span class="status-badge online">
                    <span class="status-dot"></span>
                    ${player.status}
                </span>
            </td>
            <td>
                <div class="action-buttons">
                    <button class="action-btn" onclick="playerAction('edit', ${player.id}, '${player.name}')">Edit</button>
                    <button class="action-btn" onclick="playerAction('send', ${player.id}, '${player.name}')">Send</button>
                    <button class="action-btn" onclick="playerAction('move', ${player.id}, '${player.name}')">Move</button>
                    <button class="action-btn warning" onclick="playerAction('kick', ${player.id}, '${player.name}')">Kick</button>
                    <button class="action-btn danger" onclick="playerAction('ban', ${player.id}, '${player.name}')">Ban</button>
                </div>
            </td>
        `;
        playersTbody.appendChild(row);
    });
    
    playersCount.textContent = players.length;
}

function loadPlayers() {
    fetch('/api/players')
        .then(response => response.json())
        .then(players => {
            allPlayers = players;
            renderPlayers(players);
        })
        .catch(error => {
            console.error('Error loading players:', error);
        });
}

playerSearch.addEventListener('input', (e) => {
    const searchTerm = e.target.value.toLowerCase();
    const filtered = allPlayers.filter(player => 
        player.name.toLowerCase().includes(searchTerm) ||
        player.class.toLowerCase().includes(searchTerm) ||
        player.ip.includes(searchTerm)
    );
    renderPlayers(filtered);
});

// Modal
const modal = document.getElementById('action-modal');
const modalTitle = document.getElementById('modal-title');
const modalMessage = document.getElementById('modal-message');
const modalInput = document.getElementById('modal-input');
const modalClose = document.getElementById('modal-close');
const modalCancel = document.getElementById('modal-cancel');
const modalConfirm = document.getElementById('modal-confirm');

let currentAction = null;
let currentPlayerId = null;

function playerAction(action, playerId, playerName) {
    currentAction = action;
    currentPlayerId = playerId;
    
    const actionTitles = {
        'kick': 'Kick Player',
        'ban': 'Ban Player',
        'edit': 'Edit Player',
        'send': 'Send Item',
        'move': 'Move Player'
    };
    
    const actionMessages = {
        'kick': `Are you sure you want to kick ${playerName}?`,
        'ban': `Are you sure you want to ban ${playerName}? Enter the reason:`,
        'edit': `Edit ${playerName}'s information:`,
        'send': `Send an item to ${playerName}. Enter the item ID:`,
        'move': `Move ${playerName} to another location. Enter the coordinates:`
    };
    
    modalTitle.textContent = actionTitles[action];
    modalMessage.textContent = actionMessages[action];
    
    if (action === 'kick') {
        modalInput.style.display = 'none';
    } else {
        modalInput.style.display = 'block';
        modalInput.value = '';
    }
    
    modal.classList.add('active');
}

function closeModal() {
    modal.classList.remove('active');
    currentAction = null;
    currentPlayerId = null;
}

modalClose.addEventListener('click', closeModal);
modalCancel.addEventListener('click', closeModal);

modalConfirm.addEventListener('click', () => {
    const inputValue = modalInput.value;
    
    fetch('/api/player/action', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify({
            action: currentAction,
            player_id: currentPlayerId,
            value: inputValue
        })
    })
    .then(response => response.json())
    .then(data => {
        const time = new Date().toLocaleTimeString('en-US', { hour12: false });
        addConsoleLog(time, 'success', data.message);
        closeModal();
    })
    .catch(error => {
        const time = new Date().toLocaleTimeString('en-US', { hour12: false });
        addConsoleLog(time, 'error', `Error: ${error.message}`);
    });
});

// Close the modal when clicking outside it
modal.addEventListener('click', (e) => {
    if (e.target === modal) {
        closeModal();
    }
});

// Initialize
loadPlayers();

// Refresh players automatically every 10 seconds
setInterval(loadPlayers, 10000);
