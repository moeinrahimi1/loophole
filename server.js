// server-tls.js
const net = require("net");
const tls = require("tls");
const fs = require("fs");

const TUNNEL_PORT = Number(process.env.TUNNEL_PORT || 7000);
const TARGET_HOST = process.env.TARGET_HOST || "127.0.0.1";
const TARGET_PORT = Number(process.env.TARGET_PORT || 8085);
const STATS_INTERVAL_MS = Number(process.env.STATS_INTERVAL_MS || 2000);

// TLS cert paths
const TLS_CERT = process.env.TLS_CERT || "./cert.pem";
const TLS_KEY = process.env.TLS_KEY || "./key.pem";

let connIdSeq = 0;

function setSocketOpts(sock) {
  sock.setNoDelay(true);
  sock.setKeepAlive(true, 10_000);
  sock.on("error", () => {});
}

function fmtBytes(n) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

const tlsOptions = {
  cert: fs.readFileSync(TLS_CERT),
  key: fs.readFileSync(TLS_KEY),
  requestCert: false,
  rejectUnauthorized: false,
};

const server = tls.createServer(tlsOptions, (tunnelSocket) => {
  const id = ++connIdSeq;
  setSocketOpts(tunnelSocket);

  const clientAddr = `${tunnelSocket.remoteAddress}:${tunnelSocket.remotePort}`;
  console.log(`[server#${id}] TLS tunnel connected from ${clientAddr}`);

  let inBytes = 0;
  let outBytes = 0;

  const targetSocket = net.connect({ host: TARGET_HOST, port: TARGET_PORT }, () => {
    setSocketOpts(targetSocket);
    console.log(`[server#${id}] connected to target ${TARGET_HOST}:${TARGET_PORT}`);

    tunnelSocket.on("data", (chunk) => (inBytes += chunk.length));
    targetSocket.on("data", (chunk) => (outBytes += chunk.length));

    tunnelSocket.pipe(targetSocket);
    targetSocket.pipe(tunnelSocket);
  });

  const interval = setInterval(() => {
    console.log(`[server#${id}] traffic in=${fmtBytes(inBytes)} out=${fmtBytes(outBytes)}`);
  }, STATS_INTERVAL_MS);

  function cleanup(reason) {
    clearInterval(interval);
    console.log(`[server#${id}] closed (${reason}) total in=${fmtBytes(inBytes)} out=${fmtBytes(outBytes)}`);
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
  console.log(`[server] TLS tunnel listening on 0.0.0.0:${TUNNEL_PORT} -> ${TARGET_HOST}:${TARGET_PORT}`);
});
