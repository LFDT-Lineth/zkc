;;error:4:31-32:not permitted in pure context
;;error:4:22-33:expected bool, found u16
(defcolumns (X :i16))
(defconstraint c1 () (shift X X))
