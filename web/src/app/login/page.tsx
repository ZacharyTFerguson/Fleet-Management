import { DeskNav } from "@/components/DeskNav";
import { LoginForm } from "@/components/LoginForm";

export default function LoginPage() {
  return (
    <main className="shell">
      <div className="atmosphere" aria-hidden="true" />
      <div className="grain" aria-hidden="true" />
      <DeskNav current="login" />
      <header className="hero">
        <p className="brand">FLEET</p>
        <h1 className="headline">Desk login</h1>
        <p className="lede">
          First sign-in creates the operator when no desk user exists. Passwords
          stay on the server. Secrets and OneStep sends require this session.
        </p>
      </header>
      <LoginForm />
    </main>
  );
}
