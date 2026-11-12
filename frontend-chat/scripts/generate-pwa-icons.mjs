import { mkdir, readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import sharp from "sharp";

// Reuse the app's vector identity. Fill the corners for OS icon masks.
const source = await readFile(new URL("../src/app/icon.svg", import.meta.url), "utf8");
const opaque = Buffer.from(source.replace('rx="14"', 'rx="0"'));
const output = new URL("../public/icons/", import.meta.url);
await mkdir(output, { recursive: true });
for (const [name, size] of [
  ["icon-192.png", 192],
  ["icon-512.png", 512],
  ["maskable-512.png", 512],
  ["apple-touch-icon.png", 180],
]) {
  // Padding keeps every part of the glyph inside the maskable safe circle.
  const image = await sharp(opaque)
    .resize(Math.round(size * 0.8))
    .png()
    .toBuffer();
  await sharp({ create: { width: size, height: size, channels: 4, background: "#2563eb" } })
    .composite([{ input: image, gravity: "centre" }])
    .png()
    .toFile(fileURLToPath(new URL(name, output)));
}
