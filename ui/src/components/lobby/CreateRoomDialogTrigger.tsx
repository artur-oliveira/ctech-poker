'use client';

import dynamic from 'next/dynamic';
import {Lock} from 'lucide-react';
import {useState} from 'react';
import {Button} from '@/components/ui/button';

// Room configuration brings React Hook Form and Zod with it. Players scan and
// join public tables far more often than they create a private one, so keep
// that validation stack off the lobby's interaction-critical path. The first
// press starts the download and opens the dialog as soon as it is ready.
const CreateRoomDialog = dynamic(() => import('./CreateRoomDialog').then(module => module.CreateRoomDialog), {
  ssr: false,
  loading: () => <Button size="lg" variant="outline" disabled aria-live="polite">
    <Lock aria-hidden="true"/>Preparando mesa…
  </Button>
});

export function CreateRoomDialogTrigger() {
  const [requested, setRequested] = useState(false);
  if (requested) return <CreateRoomDialog initialOpen/>;
  return <Button size="lg" variant="outline" onClick={() => setRequested(true)}>
    <Lock aria-hidden="true"/>Mesa privada
  </Button>;
}
