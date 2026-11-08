// Entry of the browser live demo (`npm run build:demo`). The storage guard must
// run before the app reads or writes anything.
import "./mock/storage";
import "./mock/narrow";
import "./main";
