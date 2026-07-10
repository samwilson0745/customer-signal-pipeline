export type Channel = "twitter" | "chat" | "email" | "app_review";
export type Tone = "positive" | "neutral" | "negative" | "urgent";

export interface MessageTemplate {
  tone: Tone;
  text: string;
}

// A bank of realistic-ish message templates per channel, spanning the tones
// the downstream LLM classifier is expected to distinguish between.
export const MESSAGE_BANK: Record<Channel, MessageTemplate[]> = {
  twitter: [
    { tone: "urgent", text: "@{brand} your app has been down for 45 minutes and I'm losing sales. FIX THIS NOW." },
    { tone: "negative", text: "Been on hold with @{brand} support for over an hour. Absolutely ridiculous service." },
    { tone: "negative", text: "@{brand} charged me twice this month and nobody will explain why. Not happy." },
    { tone: "neutral", text: "Does @{brand} have a way to export my data to CSV? Can't find it in settings." },
    { tone: "neutral", text: "@{brand} when is the next feature update coming? Curious what's on the roadmap." },
    { tone: "positive", text: "Just switched to @{brand} and the onboarding experience was seamless. Great work!" },
    { tone: "positive", text: "Shoutout to @{brand} support for resolving my issue in under 10 minutes today." },
    { tone: "urgent", text: "URGENT: @{brand} payment portal is throwing 500 errors for everyone on my team right now." },
  ],
  chat: [
    { tone: "urgent", text: "Hi, our production integration with {brand} just stopped working and we're losing customers by the minute. Need help ASAP." },
    { tone: "negative", text: "I've asked three times for a refund and keep getting bounced between agents. This is frustrating." },
    { tone: "neutral", text: "Can you walk me through how to set up SSO for our organization?" },
    { tone: "neutral", text: "What's the difference between the Pro and Enterprise plans?" },
    { tone: "positive", text: "Thanks so much, that fixed it! Really appreciate the quick turnaround." },
    { tone: "positive", text: "Your dashboard redesign is so much cleaner, nice job team." },
    { tone: "negative", text: "The mobile app keeps crashing every time I try to upload a photo. Very annoying." },
    { tone: "urgent", text: "We think there's been unauthorized access to our account, please escalate immediately." },
  ],
  email: [
    { tone: "neutral", text: "Hello, I'd like to request an invoice for our last billing cycle. Thanks in advance." },
    { tone: "negative", text: "Subject: Disappointed with recent outage. We experienced a full day of downtime with no proactive communication from your team." },
    { tone: "urgent", text: "Subject: CRITICAL - Data sync failure impacting production. Our nightly sync with {brand} failed for the third night in a row and it's affecting live reporting." },
    { tone: "positive", text: "Subject: Thank you! Wanted to send a note thanking your onboarding specialist for going above and beyond." },
    { tone: "neutral", text: "Subject: Question about API rate limits. Could you clarify the rate limits on the v2 endpoints?" },
    { tone: "negative", text: "Subject: Billing discrepancy. I was billed for a plan tier I downgraded from two months ago." },
    { tone: "positive", text: "Subject: Renewal feedback. Renewing for another year, the platform has genuinely improved our workflow." },
  ],
  app_review: [
    { tone: "positive", text: "5 stars, this app has completely replaced three other tools we used to juggle. Love it." },
    { tone: "negative", text: "2 stars. Constant crashes since the last update, please fix." },
    { tone: "neutral", text: "3 stars, does what it says but the UI feels dated compared to competitors." },
    { tone: "urgent", text: "1 star. App wiped all my saved data after the update, this is a disaster for my business." },
    { tone: "positive", text: "5 stars, customer support responded within minutes and solved my issue instantly." },
    { tone: "negative", text: "1 star, keeps logging me out every few minutes, unusable at this point." },
    { tone: "neutral", text: "4 stars, solid app overall, would like to see dark mode added soon." },
  ],
};

const AUTHOR_PREFIXES = [
  "alex", "jordan", "sam", "taylor", "morgan", "casey", "riley", "jamie",
  "devon", "quinn", "avery", "sage", "reese", "harper", "rowan", "kai",
];

export function randomAuthor(channel: Channel): string {
  const prefix = AUTHOR_PREFIXES[Math.floor(Math.random() * AUTHOR_PREFIXES.length)];
  const suffix = Math.floor(Math.random() * 9000) + 1000;
  if (channel === "twitter") return `@${prefix}${suffix}`;
  if (channel === "app_review") return `${prefix}${suffix}`;
  return `${prefix}.${suffix}@example.com`;
}

export function randomTemplate(channel: Channel): MessageTemplate {
  const bank = MESSAGE_BANK[channel];
  return bank[Math.floor(Math.random() * bank.length)];
}

export const CHANNELS: Channel[] = ["twitter", "chat", "email", "app_review"];
