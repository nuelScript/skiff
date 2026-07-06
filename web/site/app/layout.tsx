import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";

const sans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const mono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

const title = "Skiff — Ship it to a server you own";
const description =
  "Push-to-deploy with automatic HTTPS, managed databases, and preview environments — running on infrastructure you control, not rented.";

export const metadata: Metadata = {
  metadataBase: new URL("https://useskiff.xyz"),
  title,
  description,
  openGraph: {
    title,
    description,
    url: "https://useskiff.xyz",
    siteName: "Skiff",
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title,
    description,
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${sans.variable} ${mono.variable} antialiased`}
    >
      <body>{children}</body>
    </html>
  );
}
