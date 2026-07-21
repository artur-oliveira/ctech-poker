'use client'
import {useEffect, useState} from 'react'
import {CircleAlert, X} from 'lucide-react'
import {type AppNotification, dismissNotification, subscribeNotifications} from '@/lib/notify'

export function Notifier() {
  const [items, setItems] = useState<AppNotification[]>([])
  useEffect(() => subscribeNotifications(setItems), [])
  if (!items.length) return null
  return (
    <div className="api-notifier" role="region" aria-label="Avisos" aria-live="assertive">
      {items.map(n => (
        <div key={n.id} className={`api-toast ${n.variant}`} role="alert">
          <CircleAlert aria-hidden="true"/>
          <p>{n.message}</p>
          <button type="button" aria-label="Fechar aviso" onClick={() => dismissNotification(n.id)}><X/></button>
        </div>
      ))}
    </div>
  )
}
