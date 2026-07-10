import { Kafka, Producer, logLevel } from "kafkajs";
import { CHANNELS, randomAuthor, randomTemplate } from "./messageBank";

const KAFKA_BROKERS = (process.env.KAFKA_BROKERS ?? "localhost:9092").split(",");
const KAFKA_TOPIC = process.env.KAFKA_TOPIC ?? "customer-signals";
const EVENTS_PER_SECOND = Number(process.env.EVENTS_PER_SECOND ?? 2);
const BRANDS = (process.env.BRANDS ?? "acme").split(",").map((b) => b.trim()).filter(Boolean);

if (EVENTS_PER_SECOND <= 0) {
  throw new Error("EVENTS_PER_SECOND must be a positive number");
}
if (BRANDS.length === 0) {
  throw new Error("BRANDS must contain at least one brand name");
}

const kafka = new Kafka({
  clientId: "customer-signal-producer",
  brokers: KAFKA_BROKERS,
  logLevel: logLevel.ERROR,
  retry: {
    initialRetryTime: 300,
    retries: 10,
  },
});

let seq = 0;

function buildEvent(): { event_id: string; channel: string; author: string; text: string; lang: string; created_at: string; brand: string } {
  const channel = CHANNELS[Math.floor(Math.random() * CHANNELS.length)];
  const brand = BRANDS[Math.floor(Math.random() * BRANDS.length)];
  const template = randomTemplate(channel);
  const text = template.text.replace(/{brand}/g, brand);
  seq += 1;
  return {
    event_id: `evt_${Date.now()}_${seq}`,
    channel,
    author: randomAuthor(channel),
    text,
    lang: "en",
    created_at: new Date().toISOString(),
    brand,
  };
}

function log(msg: string, extra: Record<string, unknown> = {}): void {
  console.log(JSON.stringify({ level: "info", service: "producer", msg, ...extra, ts: new Date().toISOString() }));
}

function logError(msg: string, err: unknown): void {
  console.error(JSON.stringify({ level: "error", service: "producer", msg, error: String(err), ts: new Date().toISOString() }));
}

async function run(): Promise<void> {
  const producer: Producer = kafka.producer({ allowAutoTopicCreation: true });
  await producer.connect();
  log("connected to kafka", { brokers: KAFKA_BROKERS, topic: KAFKA_TOPIC, eventsPerSecond: EVENTS_PER_SECOND, brands: BRANDS });

  const intervalMs = 1000 / EVENTS_PER_SECOND;

  const emit = async (): Promise<void> => {
    const event = buildEvent();
    try {
      await producer.send({
        topic: KAFKA_TOPIC,
        messages: [{ key: event.brand, value: JSON.stringify(event) }],
      });
      log("published event", { event_id: event.event_id, channel: event.channel, brand: event.brand });
    } catch (err) {
      logError("failed to publish event", err);
    }
  };

  const timer = setInterval(() => {
    emit().catch((err) => logError("unexpected emit error", err));
  }, intervalMs);

  const shutdown = async (signal: string): Promise<void> => {
    log("shutting down", { signal });
    clearInterval(timer);
    await producer.disconnect();
    process.exit(0);
  };

  process.on("SIGINT", () => void shutdown("SIGINT"));
  process.on("SIGTERM", () => void shutdown("SIGTERM"));
}

run().catch((err) => {
  logError("fatal producer error", err);
  process.exit(1);
});
