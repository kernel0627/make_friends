import { Navigate, useLocation } from "react-router-dom";
import { getSession } from "../lib/session";

export default function ProtectedRoute({ children }) {
  const location = useLocation();
  const session = getSession();

  if (!session?.token || session?.user?.role !== "admin") {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  }

  return children;
}
