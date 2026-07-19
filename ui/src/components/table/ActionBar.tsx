'use client'
import {useState} from 'react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';

export function ActionBar({onAct}: { onAct: (a: string, n?: number) => void }) {
  const [amount, setAmount] = useState(100);
  return <div className="action-bar">
    <Button variant="outline" onClick={() => onAct('fold')}>Desistir</Button>
    <Button variant="outline" onClick={() => onAct('check')}>Mesa</Button>
    <Button variant="outline" onClick={() => onAct('call')} className="call">Pagar</Button>
    <label><Input aria-label="Valor do aumento" type="range" min="25" max="5000" step="25" value={amount} onChange={e => setAmount(+e.target.value)}/><span>{amount}</span></label>
    <Button onClick={() => onAct('raise', amount)} className="raise">Aumentar</Button>
  </div>
}
