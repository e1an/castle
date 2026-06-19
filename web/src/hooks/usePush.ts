import { useEffect, useState } from 'react';

export type PushStatus = 'unsupported' | 'blocked' | 'subscribed' | 'unsubscribed';

function urlBase64ToUint8Array(base64: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (base64.length % 4)) % 4);
  const b64 = (base64 + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(b64);
  const arr = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) arr[i] = raw.charCodeAt(i);
  return arr;
}

function isSupported(): boolean {
  return (
    typeof window !== 'undefined' &&
    'serviceWorker' in navigator &&
    'PushManager' in window &&
    'Notification' in window
  );
}

export function usePush() {
  const [status, setStatus] = useState<PushStatus>(() => {
    if (!isSupported()) return 'unsupported';
    if (Notification.permission === 'denied') return 'blocked';
    return 'unsubscribed';
  });

  useEffect(() => {
    if (!isSupported() || Notification.permission === 'denied') return;
    navigator.serviceWorker.ready
      .then((reg) => reg.pushManager.getSubscription())
      .then((sub) => setStatus(sub ? 'subscribed' : 'unsubscribed'))
      .catch(() => setStatus('unsupported'));
  }, []);

  async function subscribe() {
    try {
      const permission = await Notification.requestPermission();
      if (permission !== 'granted') {
        setStatus('blocked');
        return;
      }
      const reg = await navigator.serviceWorker.ready;
      const res = await fetch('/api/push/vapid-public-key');
      const { public_key } = await res.json() as { public_key: string };

      const sub = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(public_key),
      });
      const json = sub.toJSON();
      await fetch('/api/push/subscribe', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          endpoint: json.endpoint,
          p256dh: json.keys?.p256dh ?? '',
          auth: json.keys?.auth ?? '',
        }),
      });
      setStatus('subscribed');
    } catch (err) {
      console.error('Push subscribe failed:', err);
    }
  }

  async function unsubscribe() {
    try {
      const reg = await navigator.serviceWorker.ready;
      const sub = await reg.pushManager.getSubscription();
      if (!sub) { setStatus('unsubscribed'); return; }
      await fetch('/api/push/subscribe', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ endpoint: sub.endpoint }),
      });
      await sub.unsubscribe();
      setStatus('unsubscribed');
    } catch (err) {
      console.error('Push unsubscribe failed:', err);
    }
  }

  return { status, subscribe, unsubscribe };
}
