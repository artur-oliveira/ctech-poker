'use client';
import {useState} from 'react';
import {Check, Copy, Share2} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger} from '@/components/ui/dialog';

export function InviteDialog({url}: { url: string }) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard API unavailable/blocked — the input stays visible and selectable for a manual copy.
    }
  }

  async function share() {
    if (navigator.share) {
      try {
        await navigator.share({url});
      } catch {
        // User dismissed the native share sheet — nothing to recover from.
      }
      return;
    }
    await copy();
  }

  return <Dialog>
    <DialogTrigger render={<Button type="button" variant="ghost" size="icon" aria-label="Convidar para a mesa"/>}>
      <Share2/>
    </DialogTrigger>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Convidar para a mesa</DialogTitle>
        <DialogDescription>Compartilhe este link para chamar alguém para esta mesa.</DialogDescription>
      </DialogHeader>
      <div className="flex items-center gap-2">
        <Input readOnly value={url} aria-label="Link de convite" onFocus={event => event.currentTarget.select()}/>
        <Button type="button" onClick={share}>
          {copied ? <><Check/> Copiado</> : <><Copy/> Copiar</>}
        </Button>
      </div>
    </DialogContent>
  </Dialog>;
}
