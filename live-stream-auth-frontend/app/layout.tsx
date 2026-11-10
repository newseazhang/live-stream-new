import type { ReactNode } from 'react';
import './globals.css';

export const metadata = {
  title: '直播大厅 · Live Stream',
  description: '创建直播间、开播，发现正在直播的房间。',
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="zh-CN">
      <body>{children}</body>
    </html>
  );
}
