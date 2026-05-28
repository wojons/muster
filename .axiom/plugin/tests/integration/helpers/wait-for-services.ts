/**
 * Wait for integration test services to be ready.
 * Used by Tier 3 integration tests to ensure Docker services are up.
 *
 * axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md plan=phase-2/task-2.1 jira_ref=DEX-434
 */

export const NATS_URL = process.env.NATS_URL ?? "nats://localhost:4222";
export const KAFKA_BROKERS = (process.env.KAFKA_BROKERS ?? "localhost:9092").split(",");
export const AWS_ENDPOINT_URL = process.env.AWS_ENDPOINT_URL ?? "http://localhost:4566";
export const AWS_REGION = process.env.AWS_REGION ?? "us-east-1";

/** Check if a TCP port is open. */
async function isPortOpen(host: string, port: number): Promise<boolean> {
    return new Promise((resolve) => {
        const net = require("node:net");
        const socket = new net.Socket();
        socket.setTimeout(1000);
        socket.on("connect", () => { socket.destroy(); resolve(true); });
        socket.on("error", () => { socket.destroy(); resolve(false); });
        socket.on("timeout", () => { socket.destroy(); resolve(false); });
        socket.connect(port, host);
    });
}

/** Wait for a service to be ready (port open). */
export async function waitForService(
    name: string,
    host: string,
    port: number,
    timeoutMs = 30_000,
): Promise<void> {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
        if (await isPortOpen(host, port)) return;
        await new Promise((r) => setTimeout(r, 500));
    }
    throw new Error(`${name} not ready at ${host}:${port} after ${timeoutMs}ms — is Docker running? Run: docker compose -f docker-compose.listen-notify-test.yml up -d --wait`);
}

/** Skip test if integration services are not available. */
export async function skipIfServicesUnavailable(services: Array<{ name: string; host: string; port: number }>): Promise<boolean> {
    for (const svc of services) {
        if (!(await isPortOpen(svc.host, svc.port))) {
            console.warn(`⚠️  Skipping integration test: ${svc.name} not available at ${svc.host}:${svc.port}`);
            return true;
        }
    }
    return false;
}

/**
 * Skip test if a required npm package is not installed.
 * Use this alongside skipIfServicesUnavailable when the adapter depends on
 * an optional peer dependency (e.g. kafkajs, @nats-io/nats.js).
 */
export async function skipIfPackageUnavailable(packageName: string): Promise<boolean> {
    try {
        await import(packageName);
        return false;
    } catch {
        console.warn(`⚠️  Skipping integration test: package '${packageName}' not installed`);
        return true;
    }
}
