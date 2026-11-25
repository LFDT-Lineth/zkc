;;error:3:17-18:non-binary selector encountered
(defcolumns (P :u2) (X :u16) (Y :u16))
(deflookup l1 ((P Y)) ((_ X)))
