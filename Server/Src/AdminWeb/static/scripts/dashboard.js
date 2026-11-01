let chatNation = 'all';
let currentLog = 'gameserver';  // Inicialmente exibe logs do GameServer

// Função para buscar dados JSON
async function fetchJSON(url) {
  const r = await fetch(url, { cache: 'no-store' });
  return await r.json();
}

// Atualiza o status do servidor, mysql e hora
async function refreshStatus() {
  try {
    const s = await fetchJSON('/api/status');
    document.getElementById('srv-status').textContent = s.server_status;
    document.getElementById('mysql-status').textContent = s.mysql ? 'Online ✅' : 'Offline ❌';
    document.getElementById('server-time').textContent = s.server_time;
  } catch (e) {}
}

// Renderiza a tabela de players
function renderTable(tbodyId, rows) {
  const tb = document.getElementById(tbodyId);
  tb.innerHTML = '';
  rows.forEach(r => {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td>${r.login}</td><td>${r.character}</td><td>${r.ip}</td>`;
    tb.appendChild(tr);
  });
}

// Atualiza a lista de jogadores por nação
async function refreshPlayers() {
  try {
    const p = await fetchJSON('/api/players');
    renderTable('tbody-exion', p.exion || []);
    renderTable('tbody-lenaria', p.lenaria || []);
    renderTable('tbody-ostyrion', p.ostyrion || []);
  } catch (e) {}
}

// Atualiza os logs do servidor
async function refreshLogs() {
  try {
    const l = await fetchJSON('/api/logs');
    const logs = l.logs || [];

    // Exibe os logs na aba ativa
    const logBox = document.getElementById(`console-${currentLog}`);
    logBox.innerHTML = logs.map(log => `<div class="line">${log}</div>`).join('');
    logBox.scrollTop = logBox.scrollHeight;
  } catch (e) {}
}

// Atualiza o chat com base na nação selecionada
async function refreshChat() {
  try {
    const c = await fetchJSON('/api/chat?nation=' + chatNation);
    const box = document.getElementById('chat-box');
    box.innerHTML = (c.messages || []).map(x => `<div class="line">${x}</div>`).join('');
    box.scrollTop = box.scrollHeight;
  } catch (e) {}
}

// Troca de aba de chat (Nação)
function switchChatTab(btn) {
  document.querySelectorAll('.tab').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  chatNation = btn.getAttribute('data-nation');
  refreshChat();
}

// Enviar comando para o servidor
async function sendCommand() {
  const input = document.getElementById('cmd-input');
  const cmd = input.value.trim();
  if (!cmd) return;
  try {
    await fetch('/api/console', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ cmd })
    });
    input.value = '';
    refreshLogs();
  } catch (e) {}
}

// Limpar logs do console
function clearLogs() {
  document.getElementById(`console-${currentLog}`).innerHTML = '';
}

let isProcessing = false;  // Variável de controle para evitar múltiplos cliques

async function controlServer(action) {
  if (isProcessing) return;  // Evita múltiplos cliques

  isProcessing = true;  // Marca que está processando

  try {
    const response = await fetch('/api/server/control', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action })
    });

    const data = await response.json();
    if (data.ok) {
      alert(`${action} do servidor bem-sucedido!`);
    } else {
      alert(`Falha: ${data.error || 'Erro desconhecido'}`);
    }

    // Atualiza o status e logs
    refreshStatus();
    refreshLogs();
  } catch (e) {
    alert('Erro de rede');
  }

  isProcessing = false;  // Libera o botão novamente após o processamento
}

async function refreshLogs() {
  try {
    const l = await fetchJSON('/api/logs');
    console.log(l.logs);  // Adicione para ver o que está sendo retornado
    const box = document.getElementById('console-box');
    box.innerHTML = (l.logs || []).map(x => `<div class="line">${x}</div>`).join('');
    box.scrollTop = box.scrollHeight;
  } catch (e) {
    console.error('Erro ao buscar logs:', e);
  }
}


// Função para alternar entre os consoles de log (GameServer, TokenServer, LoginServer)
function switchLogTab(btn) {
  document.querySelectorAll('.tab').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  currentLog = btn.getAttribute('data-log');

  // Esconde todos os logs e exibe o log da aba ativa
  document.querySelectorAll('[id^="console-"]').forEach(div => div.style.display = 'none');
  document.getElementById(`console-${currentLog}`).style.display = 'block';

  refreshLogs();  // Atualiza os logs ao trocar de aba
}

// Atualizar informações imediatamente ao carregar
refreshStatus();
refreshPlayers();
refreshLogs();
refreshChat();

// Intervalos de atualização
setInterval(refreshStatus, 2000);
setInterval(refreshPlayers, 4000);
setInterval(refreshLogs, 2000);
setInterval(refreshChat, 2000);
// Fim do dashboard.js

// ===== WEBSOCKET DO PAINEL ADMIN (Flask + Socket.IO) =====
// Conecta ao backend WebSocket (painel), não ao GameServer TCP
const socket = io("/admin"); // mesmo domínio, namespace definido no backend

socket.on("connect", () => {
  console.log("[WS] conectado ao painel");
  
  // Solicita atualização inicial
  socket.emit("request_status");
  socket.emit("request_players", { channel: 1 });
});

// Atualização de status (CPU / RAM / LoginServer / Channels)
socket.on("status", (data) => {
  try {
    document.getElementById('srv-status').textContent = data.login_server;
    if (data.cpu !== undefined) document.getElementById('cpu-usage').textContent = data.cpu + "%";
    if (data.ram !== undefined) document.getElementById('ram-usage').textContent = data.ram + "%";
  } catch(e){}
});

// Atualização de jogadores do canal atual
socket.on("players", (data) => {
  const list = data.players || [];
  const tbody = document.getElementById("tbody-current-channel");
  if (tbody) {
    tbody.innerHTML = "";
    list.forEach(p => {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td>${p.cid}</td><td>${p.name}</td><td>${p.level}</td><td>${p.nation}</td>`;
      tbody.appendChild(tr);
    });
  }
});

// Logs enviados automaticamente (GameServer / LoginServer / TokenServer)
socket.on("log_message", (data) => {
  const logBox = document.getElementById('console-' + currentLog);
  if (!logBox) return;
  const div = document.createElement('div');
  div.classList.add('line');
  div.textContent = `[${data.server}] ${data.log}`;
  logBox.appendChild(div);
  logBox.scrollTop = logBox.scrollHeight;
});

// Enviar broadcast via WebSocket
function wsBroadcast(message) {
  socket.emit("command", { type: "broadcast", channel: 1, message });
}
