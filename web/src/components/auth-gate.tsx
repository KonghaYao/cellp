import { useCallback, useState } from "react";
import { hasAdminToken } from "@/lib/auth";
import { LoginPage } from "@/pages/LoginPage";

interface AuthGateProps {
  children: React.ReactNode;
}

export function AuthGate({ children }: AuthGateProps) {
  const [authenticated, setAuthenticated] = useState(hasAdminToken);

  const handleAuthenticated = useCallback(() => {
    setAuthenticated(true);
  }, []);

  if (!authenticated) {
    return <LoginPage onAuthenticated={handleAuthenticated} />;
  }

  return <>{children}</>;
}
