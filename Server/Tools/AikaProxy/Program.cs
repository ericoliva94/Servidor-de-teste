// AikaProxy: fica entre o cliente e os canais do AikaServer.exe pre-compilado.
//
// O binario atual trata so o primeiro pacote de cada recv(): se o cliente manda
// dois pacotes no mesmo segmento TCP o segundo e descartado, e pacotes partidos
// sao processados pela metade. O proxy separa o fluxo em pacotes completos
// ([ushort tamanho][...]) e entrega um por vez, com um intervalo minimo entre
// eles, para que cada recv() do servidor receba exatamente um pacote.
//
// Quando o servidor for recompilado com Src\Threads\PlayerThread.pas corrigido,
// o proxy deixa de ser necessario (mas continua funcionando).

using System.Net;
using System.Net.Sockets;
using System.Text.Json;

const int HeaderSize = 12;
const int MaxPacketSize = 22000;

var configPath = Path.Combine(AppContext.BaseDirectory, "AikaProxy.json");
var config = JsonSerializer.Deserialize<ProxyConfig>(File.ReadAllText(configPath),
    new JsonSerializerOptions { PropertyNameCaseInsensitive = true }) ?? new ProxyConfig();
var gap = TimeSpan.FromMilliseconds(config.GapMs);

Console.Title = "Aika Proxy";
var listeners = new List<Task>();
foreach (var route in config.Routes)
{
    var listener = new TcpListener(IPEndPoint.Parse(route.Listen));
    listener.Start();
    Log($"escutando {route.Listen} -> {route.Target} (intervalo {config.GapMs} ms)");
    listeners.Add(AcceptLoop(listener, IPEndPoint.Parse(route.Target)));
}

await Task.WhenAll(listeners);

async Task AcceptLoop(TcpListener listener, IPEndPoint target)
{
    while (true)
    {
        var client = await listener.AcceptTcpClientAsync();
        _ = Task.Run(() => HandleClient(client, target));
    }
}

async Task HandleClient(TcpClient client, IPEndPoint target)
{
    var remote = client.Client.RemoteEndPoint?.ToString() ?? "?";
    using var server = new TcpClient();
    try
    {
        client.NoDelay = true;
        server.NoDelay = true;
        await server.ConnectAsync(target);
        Log($"{remote} conectado -> {target}");

        var clientStream = client.GetStream();
        var serverStream = server.GetStream();

        // servidor -> cliente: so repassa (ler rapido evita o buffer de envio do
        // servidor encher, caso em que o binario atual descarta pacotes)
        var down = serverStream.CopyToAsync(clientStream);
        var up = ClientToServer(clientStream, serverStream, remote);
        await Task.WhenAny(down, up);
    }
    catch (Exception e) when (e is IOException or SocketException or ObjectDisposedException)
    {
    }
    catch (Exception e)
    {
        Log($"{remote} erro: {e.Message}");
    }
    finally
    {
        client.Close();
        server.Close();
        Log($"{remote} desconectado");
    }
}

async Task ClientToServer(NetworkStream from, NetworkStream to, string remote)
{
    var pending = new byte[65536];
    var len = 0;
    var helloDone = false;
    var passthrough = false;
    var lastSend = DateTime.MinValue;

    while (true)
    {
        if (len == pending.Length)
            Array.Resize(ref pending, pending.Length * 2);

        var n = await from.ReadAsync(pending.AsMemory(len));
        if (n <= 0)
            return;

        if (passthrough)
        {
            await to.WriteAsync(pending.AsMemory(len, n));
            continue;
        }

        len += n;

        if (!helloDone)
        {
            if (len < 4)
                continue;

            // O servidor espera o "hello" de 4 bytes sozinho ou colado num 1o pacote
            // grande; enviando-o sempre sozinho o pacote seguinte nunca e cortado.
            byte[] hello;
            if (IsHello(pending, len))
            {
                hello = pending[..4];
                Consume(pending, ref len, 4);
            }
            else
            {
                hello = [0x11, 0xF3, 0x11, 0x1F];
            }

            await to.WriteAsync(hello);
            lastSend = DateTime.UtcNow;
            helloDone = true;
        }

        while (len >= HeaderSize)
        {
            int size = BitConverter.ToUInt16(pending, 0);
            if (size < HeaderSize || size > MaxPacketSize)
            {
                // formato desconhecido (ex.: cabecalho mascarado do cliente AikaBR 2024):
                // repassa sem mexer, nunca fica pior do que conectar direto
                Log($"{remote} tamanho de pacote invalido ({size}), repassando sem separar");
                await to.WriteAsync(pending.AsMemory(0, len));
                len = 0;
                passthrough = true;
                break;
            }

            if (len < size)
                break;

            var wait = gap - (DateTime.UtcNow - lastSend);
            if (wait > TimeSpan.Zero)
                await Task.Delay(wait);

            await to.WriteAsync(pending.AsMemory(0, size));
            lastSend = DateTime.UtcNow;
            Consume(pending, ref len, size);
        }
    }
}

static void Consume(byte[] buffer, ref int len, int count)
{
    Buffer.BlockCopy(buffer, count, buffer, 0, len - count);
    len -= count;
}

static bool IsHello(byte[] data, int len)
{
    // vistos: 11 F3 11 1F e ?? ?? 11 1F
    if (data[0] == 0x11 && data[1] == 0xF3) return true;
    if (data[2] == 0x11 && data[3] == 0x1F) return true;
    if (len == 4) return true;

    var size0 = BitConverter.ToUInt16(data, 0);
    if (size0 >= HeaderSize && size0 <= len) return false;
    if (len < 4 + HeaderSize) return false;
    var size4 = BitConverter.ToUInt16(data, 4);
    return size4 >= HeaderSize && size4 <= len - 4;
}

static void Log(string message) =>
    Console.WriteLine($"{DateTime.Now:dd/MM/yyyy HH:mm:ss} {message}");

class ProxyConfig
{
    public int GapMs { get; set; } = 40;
    public List<Route> Routes { get; set; } = [];
}

class Route
{
    public string Listen { get; set; } = "";
    public string Target { get; set; } = "";
}
