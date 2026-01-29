// server.js
const net = require("net");

const TUNNEL_PORT = Number(process.env.TUNNEL_PORT || 7000);
const TARGET_HOST = process.env.TARGET_HOST || "127.0.0.1";
const TARGET_PORT = Number(process.env.TARGET_PORT || 8085);
const STATS_INTERVAL_MS = Number(process.env.STATS_INTERVAL_MS || 2000);

let connIdSeq = 0;

function setSocketOpts(sock) {
  sock.setNoDelay(true);
  sock.setKeepAlive(true, 10_000);
  sock.on("error", (e) => {
    // keep logs minimal; errors happen during disconnects
  });
}

function fmtBytes(n) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

const server = net.createServer((tunnelSocket) => {
  const id = ++connIdSeq;
  setSocketOpts(tunnelSocket);

  const clientAddr = `${tunnelSocket.remoteAddress}:${tunnelSocket.remotePort}`;
  console.log(`[server#${id}] tunnel connected from ${clientAddr}`);

  let inBytes = 0;  // tunnel -> target
  let outBytes = 0; // target -> tunnel

  const targetSocket = net.connect({ host: TARGET_HOST, port: TARGET_PORT }, () => {
    setSocketOpts(targetSocket);
    console.log(
      `[server#${id}] connected to target ${TARGET_HOST}:${TARGET_PORT}`
    );

    // Count bytes tunnel -> target
    tunnelSocket.on("data", (chunk) => {
      inBytes += chunk.length;
    });

    // Count bytes target -> tunnel
    targetSocket.on("data", (chunk) => {
      outBytes += chunk.length;
    });

    // Pipe data both ways
    tunnelSocket.pipe(targetSocket);
    targetSocket.pipe(tunnelSocket);
  });

  const interval = setInterval(() => {
    console.log(
      `[server#${id}] traffic in=${fmtBytes(inBytes)} out=${fmtBytes(outBytes)}`
    );
  }, STATS_INTERVAL_MS);

  function cleanup(reason) {
    clearInterval(interval);
    console.log(
      `[server#${id}] closed (${reason}) total in=${fmtBytes(inBytes)} out=${fmtBytes(outBytes)}`
    );
  }

  tunnelSocket.on("close", () => {
    targetSocket.destroy();
    cleanup("tunnel closed");
  });

  targetSocket.on("close", () => {
    tunnelSocket.destroy();
    cleanup("target closed");
  });

  targetSocket.on("error", (err) => {
    console.log(`[server#${id}] target error: ${err.message}`);
    tunnelSocket.destroy();
  });
});

server.listen(TUNNEL_PORT, "0.0.0.0", () => {
  console.log(
    `[server] tunnel listening on 0.0.0.0:${TUNNEL_PORT} -> ${TARGET_HOST}:${TARGET_PORT}`
  );
});
