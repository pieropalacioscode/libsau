window.api = async function(method, url, body) {
  const token = localStorage.getItem('token');

  try {
    const res = await fetch(url, {
      method,
      headers: {
        'Content-Type': 'application/json',
        ...(token && { Authorization: `Bearer ${token}` })
      },
      body: body ? JSON.stringify(body) : undefined
    });

    // 🔐 1. AUTH
    if (res.status === 401) {
      console.warn("🔒 Sesión expirada");
      localStorage.removeItem('token');
      window.location.href = '/login';
      return null;
    }

    // 🧾 2. VALIDAR JSON
    const contentType = res.headers.get('content-type') || '';

    if (!contentType.includes('application/json')) {
      const text = await res.text();
      console.error("❌ Respuesta NO JSON:", text.slice(0, 200));
      return null;
    }

    // 📦 3. PARSEAR
    const data = await res.json();

    // ⚠️ 4. ERROR DE BACKEND
    if (!res.ok) {
      console.error("❌ Error API:", data);
      return { error: true, data };
    }

    return { error: false, data };

  } catch (err) {
    console.error("🔥 Error de red:", err);
    return null;
  }
};