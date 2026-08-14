(defconst ONE 1)
(defcolumns (X :i16))
(defconstraint c1 () (== 0 (* X (- X ONE))))
