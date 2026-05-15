async function refreshAccessToken() {

  try {

    const res = await fetch('/auth/refresh', {
      method: 'POST',
      credentials: 'include'
    });

    if (!res.ok) {
      return null;
    }

    const data = await res.json();

    if (!data.access_token) {
      return null;
    }

    // 🔄 guardar nuevo token
    sessionStorage.setItem(
      'access_token',
      data.access_token
    );

    return data.access_token;

  } catch (err) {

    console.error('Refresh error:', err);

    return null;
  }
}
window.api = async function(method, url, body) {

  const token = sessionStorage.getItem('access_token');

  try {

    const res = await fetch(url, {
      method,
      headers: {
        'Content-Type': 'application/json',
        ...(token && {
          Authorization: `Bearer ${token}`
        })
      },
      body: body ? JSON.stringify(body) : undefined
    });

    // 🔐 sesión inválida
    if (res.status === 401) {

  // 🔄 intentar refresh automático
  const newToken = await refreshAccessToken();

  // ❌ refresh falló
  if (!newToken) {

    sessionStorage.removeItem('access_token');
    sessionStorage.removeItem('current_user');

    if (window.location.pathname !== '/login') {
      window.location.href = '/login';
    }

    return null;
  }

  // 🔁 retry request original
  const retryRes = await fetch(url, {
    method,
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${newToken}`
    },
    body: body ? JSON.stringify(body) : undefined
  });

  const retryContentType =
    retryRes.headers.get('content-type') || '';

  if (!retryContentType.includes('application/json')) {

    const text = await retryRes.text();

    console.error('Retry respuesta NO JSON:', text);

    return null;
  }

  const retryData = await retryRes.json();

  return {
    error: !retryRes.ok,
    data: retryData
  };
}
    // validar JSON
    const contentType = res.headers.get('content-type') || '';

    if (!contentType.includes('application/json')) {

      const text = await res.text();

      console.error('Respuesta NO JSON:', text);

      return null;
    }

    const data = await res.json();

    if (!res.ok) {
      return {
        error: true,
        data
      };
    }

    return {
      error: false,
      data
    };

  } catch (err) {

    console.error('Error API:', err);

    return null;
  }
};