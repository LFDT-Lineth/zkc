;;error:3:17-18:non-binary selector encountered
(defcolumns (P :u2) (X :u16) (Y :u16) (one :i1 :constant 1))
(deflookup l1 ((P Y)) ((one X)))
