;; As lookup_02, but without a source selector.
(defcolumns (A :i16) (B :i8) (X :i16) (Y :i8) (one :i1 :constant 1))
;; Inclusion A,B into X,Y
(deflookup test ((_ A B)) ((one X Y)))
