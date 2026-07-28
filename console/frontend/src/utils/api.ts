export async function safeFetch<T>(url: string): Promise<T | null> {
  try {
    const r = await fetch(url);
    if (!r.ok) {
      const text = await r.text().catch(() => '');
      console.error(`API error ${r.status} on ${url}:`, text);
      return null;
    }
    const contentType = r.headers.get('content-type') || '';
    if (!contentType.includes('application/json')) {
      console.error(`Non-JSON response from ${url}:`, contentType);
      return null;
    }
    return await r.json();
  } catch (err) {
    console.error(`Fetch failed for ${url}:`, err);
    return null;
  }
}
