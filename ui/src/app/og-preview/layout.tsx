import type {Metadata} from 'next';
import './studio.css';

export const metadata: Metadata = {
  title: 'Estúdio de imagens sociais',
  robots: {index: false, follow: false}
};

export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
