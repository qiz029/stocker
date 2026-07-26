import { createContext, useContext, useEffect, useState } from "react";
import { BrowserRouter, Navigate, Route, Routes, useNavigate } from "react-router-dom";
import { api, ApiError, User } from "./api";
import Login from "./pages/Login";
import Lobby from "./pages/Lobby";
import Room from "./pages/Room";
import Stock from "./pages/Stock";
import Reveal from "./pages/Reveal";

// Exported so page/component tests can provide a fake user.
export const UserCtxForTest = createContext<User | null>(null);
export const useUser = () => useContext(UserCtxForTest)!;

function Shell() {
  const [user, setUser] = useState<User | null>(null);
  const [checked, setChecked] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    api.get<User>("/api/me")
      .then(setUser)
      .catch(() => setUser(null))
      .finally(() => setChecked(true));
  }, []);

  if (!checked) return null;
  if (!user) {
    return <Login onAuthed={u => { setUser(u); navigate("/"); }} />;
  }
  return (
    <UserCtxForTest.Provider value={user}>
      <Routes>
        <Route path="/" element={<Lobby />} />
        <Route path="/rooms/:roomId" element={<Room />} />
        <Route path="/rooms/:roomId/i/:instrumentId" element={<Stock />} />
        <Route path="/rooms/:roomId/reveal" element={<Reveal />} />
        <Route path="*" element={<Navigate to="/" />} />
      </Routes>
    </UserCtxForTest.Provider>
  );
}

/** Redirect to login when any child API call hits a 401 (session expiry). */
export function isAuthError(e: unknown): boolean {
  return e instanceof ApiError && e.status === 401;
}

export default function App() {
  return (
    <BrowserRouter>
      <Shell />
    </BrowserRouter>
  );
}
