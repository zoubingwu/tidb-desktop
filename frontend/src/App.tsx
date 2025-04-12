import { useState } from "react";
import { Greet } from "../wailsjs/go/main/App";

import { Button } from "@/components/ui/button";

function App() {
  const [resultText, setResultText] = useState(
    "Please enter your name below 👇",
  );
  const [name, setName] = useState("");
  const updateName = (e: any) => setName(e.target.value);
  const updateResultText = (result: string) => setResultText(result);

  const greet = () => {
    Greet(name).then(updateResultText);
  };

  return (
    <div id="App">
      <div id="result" className="result">
        {resultText}
      </div>
      <div id="input" className="input-box">
        <input
          id="name"
          className="input"
          onChange={updateName}
          autoComplete="off"
          name="input"
          type="text"
        />
        <Button className="btn" onClick={greet}>
          Greet
        </Button>
      </div>
    </div>
  );
}

export default App;
