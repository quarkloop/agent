import type { NextConfig } from "next";

const natsConnectSource = cspConnectSource(
  process.env.NEXT_PUBLIC_NATS_WS_URL ?? "ws://127.0.0.1:9222",
);

const nextConfig: NextConfig = {
  reactCompiler: true,
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          {
            key: "Content-Security-Policy",
            value: [
              "default-src 'self'",
              `connect-src 'self' ${natsConnectSource}`,
              "img-src 'self' data: blob:",
              "script-src 'self' 'unsafe-inline' 'unsafe-eval'",
              "style-src 'self' 'unsafe-inline'",
              "font-src 'self' data:",
              "object-src 'none'",
              "base-uri 'self'",
              "frame-ancestors 'none'",
            ].join("; "),
          },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
          { key: "X-Content-Type-Options", value: "nosniff" },
        ],
      },
    ];
  },
};

export default nextConfig;

function cspConnectSource(value: string): string {
  const url = new URL(value);
  if (url.protocol !== "ws:" && url.protocol !== "wss:") {
    throw new Error("NEXT_PUBLIC_NATS_WS_URL must use ws:// or wss://");
  }
  return `${url.protocol}//${url.host}`;
}
