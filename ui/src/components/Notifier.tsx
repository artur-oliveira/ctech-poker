'use client';
import {useEffect, useState} from 'react';
import {CircleAlert, Info, X} from 'lucide-react';
import {type AppNotification, dismissNotification, subscribeNotifications} from '@/lib/notify';

export function Notifier() {
  const [items, setItems] = useState<AppNotification[]>([]);
  useEffect(() => subscribeNotifications(setItems), []);
  if (!items.length) return null;
  return (
    <div className="api-notifier" role="region" aria-label="Avisos">
      {items.map(n => (
        <div key={n.id} className={`api-toast ${n.variant}`}
             role={n.variant === 'error' ? 'alert' : 'status'} aria-atomic="true">
          {n.variant === 'error' ? <CircleAlert aria-hidden="true"/> : <Info aria-hidden="true"/>}
          <div className="api-toast-main">
            <p className="wrap-anywhere">{n.message}</p>
            {n.actions?.length ? <div className="api-toast-actions">
              {n.actions.map(action => <button key={action.label} type="button" onClick={() => {
                dismissNotification(n.id);
                void action.run();
              }}>{action.label}</button>)}
            </div> : null}
          </div>
          <button type="button" aria-label="Fechar aviso" onClick={() => dismissNotification(n.id)}><X/></button>
        </div>
      ))}
    </div>
  );
}
