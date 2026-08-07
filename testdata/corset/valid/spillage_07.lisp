(module m1)
(defcolumns (S1 :i16) (A :i16))
(defconstraint spills ()
  (== 0 (* S1 (* A (shift A -2)))))

(module m2)
(defcolumns (S2 :i16) (B :i16))
(defconstraint spills ()
  (== 0 (* S2 (* B (shift B 3)))))
