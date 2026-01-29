// client.js
const net = require("net");

const LOCAL_PORT = Number(process.env.LOCAL_PORT || 5000);
const SERVER_HOST = process.env.SERVER_HOST || "127.0.0.1";
const SERVER_PORT = Number(process.env.SERVER_PORT || 7000);
const STATS_INTERVAL_MS = Number(process.env.STATS_INTERVAL_MS || 2000);

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

const localServer = net.createServer((localSocket) => {
  const id = ++connIdSeq;
  setSocketOpts(localSocket);

  const localAddr = `${localSocket.remoteAddress}:${localSocket.remotePort}`;
  console.log(`[client#${id}] local connection from ${localAddr}`);

  let inBytes = 0;  // local -> tunnel
  let outBytes = 0; // tunnel -> local

  const tunnelSocket = net.connect({ host: SERVER_HOST, port: SERVER_PORT }, () => {
    setSocketOpts(tunnelSocket);

    console.log(
      `[client#${id}] tunnel connected to ${SERVER_HOST}:${SERVER_PORT}`
    );

    localSocket.on("data", (chunk) => {
      inBytes += chunk.length;
    });

    tunnelSocket.on("data", (chunk) => {
      outBytes += chunk.length;
    });

    localSocket.pipe(tunnelSocket);
    tunnelSocket.pipe(localSocket);
  });

  const interval = setInterval(() => {
    console.log(
      `[client#${id}] traffic in=${fmtBytes(inBytes)} out=${fmtBytes(outBytes)}`
    );
  }, STATS_INTERVAL_MS);

  function cleanup(reason) {
    clearInterval(interval);
    console.log(
      `[client#${id}] closed (${reason}) total in=${fmtBytes(inBytes)} out=${fmtBytes(outBytes)}`
    );
  }

  localSocket.on("close", () => {
    tunnelSocket.destroy();
    cleanup("local closed");
  });

  tunnelSocket.on("close", () => {
    localSocket.destroy();
    cleanup("tunnel closed");
  });

  tunnelSocket.on("error", (err) => {
    console.log(`[client#${id}] tunnel error: ${err.message}`);
    localSocket.destroy();
  });
});

localServer.listen(LOCAL_PORT, "127.0.0.1", () => {
  console.log(
    `[client] listening on 127.0.0.1:${LOCAL_PORT} -> tunnel ${SERVER_HOST}:${SERVER_PORT}`
  );
});
