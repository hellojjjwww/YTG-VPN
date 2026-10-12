import { createRoot } from 'react-dom/client';
import '@fontsource-variable/onest/wght.css';
import '@fontsource-variable/unbounded/wght.css';
import '@/styles/mikan-theme.css';
import '@/pages/store/StorePage.css';
import StorePage from '@/pages/store/StorePage';

const root = document.getElementById('store-root');
if (root) createRoot(root).render(<StorePage />);
