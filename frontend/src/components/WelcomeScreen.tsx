import { ConnectionFormDialog } from "./ConnectionForm";

const WelcomeScreen = () => {
  return (
    <div className="flex flex-col items-center justify-center h-full p-8 text-center">
      <h1 className="text-3xl font-semibold mb-4">Welcome to TiDB Desktop</h1>
      <p className="text-lg text-muted-foreground mb-8">
        Manage your TiDB connections and interact with your databases.
      </p>
      {/* Render the Dialog component here */}
      <ConnectionFormDialog />
    </div>
  );
};

export default WelcomeScreen;
