/*
 * WA AI Assistant — WhatsApp AI & Blast.
 * © 2026 ngertikode.id. Hak cipta dilindungi.
 * Penggunaan tunduk pada EULA (docs/EULA.md). Dilarang menjual ulang atau
 * mendistribusikan source code ini.
 */
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import './index.css'
import App from './App.tsx'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,      // anggap data segar 30 detik (kurangi refetch)
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})

createRoot(document.getElementById('root')!).render(
  <QueryClientProvider client={queryClient}>
    <App />
  </QueryClientProvider>,
)
