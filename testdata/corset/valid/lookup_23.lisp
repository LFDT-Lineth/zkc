(defcolumns (X :i16) (P :binary) (Y :i16) (Q :binary :padding 1))
;; Q holds the value of the constant selector 1
(defconstraint q_def () (== Q 1))
;; use of selector
(defclookup test P (Y) Q (X))
