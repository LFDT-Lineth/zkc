;;error:3:17-24:malformed selector
(defcolumns (P :u1) (X :u16) (Y :u16))
(deflookup l1 (((- 0 P) Y)) ((_ X)))
