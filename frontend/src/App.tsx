import { useState, useEffect } from "react";
import { services } from "wailsjs/go/models";
import { EventsOn } from "wailsjs/runtime";
import { Disconnect } from "wailsjs/go/main/App";
import WelcomeScreen from "@/components/WelcomeScreen";
import MainDataView from "@/components/MainDataView";
import { Header } from "./components/Header";

type ViewState = "welcome" | "main";

function App() {
  const [currentView, setCurrentView] = useState<ViewState>("welcome");
  const [connectionDetails, setConnectionDetails] =
    useState<services.ConnectionDetails | null>(null);

  const navigateToMain = (details: services.ConnectionDetails) => {
    setConnectionDetails(details);
    setCurrentView("main");
  };

  useEffect(() => {
    const cleanupEstablished = EventsOn(
      "connection:established",
      (details: services.ConnectionDetails) => {
        console.log("App.tsx: connection:established received", details);
        navigateToMain(details);
      },
    );
    const cleanupDisconnected = EventsOn("connection:disconnected", () => {
      console.log("App.tsx: connection:disconnected received");
      handleDisconnect();
    });

    return () => {
      cleanupEstablished();
      cleanupDisconnected();
    };
  }, []);

  const handleDisconnect = () => {
    console.log("App.tsx: Handling disconnect state update.");
    setConnectionDetails(null);
    setCurrentView("welcome");
  };

  const triggerDisconnect = () => {
    console.log("App.tsx: Triggering disconnect via UI.");
    Disconnect();
  };

  const renderView = () => {
    switch (currentView) {
      case "welcome":
        return <WelcomeScreen />;
      case "main":
        return <MainDataView onClose={triggerDisconnect} />;
      default:
        return <div>Unknown View</div>;
    }
  };

  return (
    <div id="App" className="h-screen w-screen">
      <Header
        title={
          connectionDetails
            ? `${connectionDetails?.user}@${connectionDetails?.host}:${connectionDetails?.port}`
            : "TiDB Desktop"
        }
      />
      {renderView()}
    </div>
  );
}

export default App;
