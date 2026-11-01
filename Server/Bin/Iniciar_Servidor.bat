@echo off
rem Inicia o AikaServer e o AikaProxy (que protege os canais contra perda de pacotes).
rem Feche o AikaEmu antes: os dois usam as portas 8831, 8822 e 8090.
cd /d "%~dp0"

netstat -ano | findstr /r /c:":8831 .*LISTENING" >nul
if not errorlevel 1 (
  echo A porta 8831 ja esta em uso. Feche o AikaEmu ou o AikaServer aberto e tente de novo.
  pause
  exit /b 1
)

start "Aika Server" /min "%~dp0AikaServer.exe"
rem espera os canais (127.0.0.11 e 127.0.0.12) abrirem antes do proxy
timeout /t 8 /nobreak >nul
start "Aika Proxy" /min "%~dp0AikaProxy\AikaProxy.exe"

echo Servidor iniciado.
echo   Token/site : http://localhost:8090
echo   Login      : porta 8831
echo   Canais     : 127.0.0.1:8822 e 127.0.0.2:8822 (via AikaProxy)
echo   Contas     : admin/admin (GM) e teste/teste
timeout /t 5 >nul
