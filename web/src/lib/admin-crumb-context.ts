import { createContext } from "react";

// AdminCrumbSlot is the place in the breadcrumb of the admin console where a
// page below a page of the navigation, e.g., one user below the users, names
// itself.
export const AdminCrumbSlot = createContext<HTMLElement | null>(null);
