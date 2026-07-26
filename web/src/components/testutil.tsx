import { ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";
import { UserCtxForTest } from "../App";

export function UserProviderForTest({ username, children }: { username: string; children: ReactNode }) {
  return (
    <MemoryRouter>
      <UserCtxForTest.Provider value={{ id: 1, username }}>{children}</UserCtxForTest.Provider>
    </MemoryRouter>
  );
}
